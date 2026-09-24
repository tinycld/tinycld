package notify

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// Administrators delivers a notification to every owner and admin of this
// deployment.
//
// For the things nobody chose to subscribe to and everybody responsible needs
// to know — a limit imposed from outside, a credential about to lapse, a
// state only an administrator can resolve. It is deliberately not addressed
// to one person: the owner may be on holiday, and the point is that somebody
// who can act finds out.
//
// Disabled users are skipped. They cannot act on it and, for the case that
// motivated this, one of them may be why the notice exists.
//
// Delivery is per-user and best-effort in the same sense DeliverToUser is:
// the durable notification row is what matters, and one user's failure must
// not cost another their notice. The returned count is how many were
// delivered, and the error is non-nil only when the recipients could not be
// resolved at all — the difference between "nobody was told" and "we could
// not find out who to tell".
func Administrators(app core.App, params NotifyParams) (int, error) {
	admins, err := app.FindRecordsByFilter(
		"users",
		"(role = 'owner' || role = 'admin') && disabled != true",
		"", 0, 0, nil,
	)
	if err != nil {
		return 0, fmt.Errorf("resolve administrators: %w", err)
	}

	delivered := 0
	for _, admin := range admins {
		params.UserID = admin.Id
		if err := DeliverToUser(app, params); err != nil {
			log.Warn("could not notify an administrator",
				"user", admin.Id, "type", params.Type, "err", err)
			continue
		}
		delivered++
	}

	if delivered == 0 && len(admins) > 0 {
		log.Error("no administrator could be notified",
			"type", params.Type, "candidates", len(admins))
	}
	return delivered, nil
}
