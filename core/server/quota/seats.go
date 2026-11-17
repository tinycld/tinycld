package quota

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"tinycld.org/core/appstate"
)

// A seat is a users row that is neither a guest nor disabled. Guests are free
// because they get no package access unless an override grants it; disabled
// users are free because disabling already cuts off every login. A deleted
// user is disabled, so deleting frees a seat.
const seatFilter = `role != 'guest' AND (disabled IS NULL OR disabled = FALSE)`

// maxUsersKey is the core setting a single-org deployment uses for its limit.
const maxUsersKey = "max_users"

// SeatLimitFunc resolves the seat limit. 0 means unlimited.
type SeatLimitFunc func(app core.App) int

type seatClaim struct {
	mu      sync.RWMutex
	fn      SeatLimitFunc
	claimed bool
}

var seatClaimKey = appstate.NewKey[*seatClaim]("quota.seats")

func seatClaimFor(app core.App) *seatClaim {
	return seatClaimKey.GetOrInit(app, func() *seatClaim { return &seatClaim{} })
}

// SetSeatLimit lets a supervising composition claim app's seat limit. The
// first call wins, so nothing the deployment itself controls can raise it. A
// nil resolver is ignored: it would silently mean unlimited.
func SetSeatLimit(app core.App, fn SeatLimitFunc) {
	if fn == nil {
		return
	}
	c := seatClaimFor(app)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.claimed {
		return
	}
	c.fn, c.claimed = fn, true
}

// SeatLimit is the claimed limit, else the deployment's own max_users setting.
func SeatLimit(app core.App) int {
	c := seatClaimFor(app)
	c.mu.RLock()
	fn := c.fn
	c.mu.RUnlock()

	var limit int
	if fn != nil {
		limit = fn(app)
	} else {
		limit = int(settingInt(app, maxUsersKey))
	}
	if limit < 0 {
		return 0
	}
	return limit
}

// CountSeats returns the seats in use.
func CountSeats(app core.App) (int, error) {
	var result struct {
		Count int `db:"count"`
	}
	err := app.DB().NewQuery(`SELECT COUNT(*) AS count FROM users WHERE ` + seatFilter).One(&result)
	if err != nil {
		return 0, fmt.Errorf("count seats: %w", err)
	}
	return result.Count, nil
}

func countsAsSeat(rec *core.Record) bool {
	return rec.GetString("role") != "guest" && !rec.GetBool("disabled")
}

// BindSeats refuses a users write that moves a row INTO a seat when none is
// free. Deletes, demotions and disabling are always allowed, so an org over
// its limit can always get back under it.
func BindSeats(app core.App) {
	app.OnRecordCreate("users").BindFunc(func(e *core.RecordEvent) error {
		if countsAsSeat(e.Record) {
			if err := checkSeat(e.App); err != nil {
				return err
			}
		}
		return e.Next()
	})
	app.OnRecordUpdate("users").BindFunc(func(e *core.RecordEvent) error {
		// A record never read from the DB has a blank Original() with no Id;
		// treat it as "was not a seat" so the check runs rather than being
		// skipped by a NewRecord -> Save -> Set -> Save on one object.
		orig := e.Record.Original()
		if countsAsSeat(e.Record) && (orig.Id == "" || !countsAsSeat(orig)) {
			if err := checkSeat(e.App); err != nil {
				return err
			}
		}
		return e.Next()
	})
}

// checkSeat refuses the write when the org is at its limit. It fails OPEN when
// the count cannot be read: a seat limit is a commercial limit, not a security
// boundary, and an unreadable count must not block a customer's work.
//
// 402 Payment Required: the request is well-formed and authorized; the org has
// run out of what it has.
func checkSeat(app core.App) error {
	limit := SeatLimit(app)
	if limit <= 0 {
		return nil
	}
	used, err := CountSeats(app)
	if err != nil {
		log.Warn("seat check: could not count seats; allowing the write", "error", err)
		return nil
	}
	if used < limit {
		return nil
	}
	// Info, not Warn: reaching the limit is the limit working, not an incident.
	log.Info("seat check: refused a write at the limit", "used", used, "limit", limit)
	return router.NewApiError(http.StatusPaymentRequired,
		fmt.Sprintf("User limit reached: %d of %d seats in use. Remove a user to free a seat.", used, limit),
		nil)
}
