package coreserver

import (
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/quota"
)

// SeatsResponse is the seat use the members screen shows. Limit 0 = unlimited.
type SeatsResponse struct {
	Used  int `json:"used"`
	Limit int `json:"limit"`
}

// RegisterSeatsEndpoint serves GET /api/seats. Any signed-in user may read it:
// knowing the org is at its limit is not privileged, and hiding it only
// produces confusing refusals.
func RegisterSeatsEndpoint(app *pocketbase.PocketBase) {
	registerSeatsEndpointCore(app)
}

func registerSeatsEndpointCore(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.GET("/api/seats", func(re *core.RequestEvent) error {
			used, err := quota.CountSeats(re.App)
			if err != nil {
				return re.InternalServerError("Could not count seats", err)
			}
			return re.JSON(http.StatusOK, SeatsResponse{Used: used, Limit: quota.SeatLimit(re.App)})
		}).BindFunc(requireAuthCore)
		return e.Next()
	})
}
