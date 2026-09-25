package coreserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// setOrgName renames the deployment. The name lives in Meta.AppName because
// /api/org-info already serves it before login; there is no other store.
func setOrgName(app core.App, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("Name is required.")
	}
	if len(name) > 255 {
		return errors.New("Name must be 255 characters or fewer.")
	}
	app.Settings().Meta.AppName = name
	return app.Save(app.Settings())
}

// RegisterOrgNameEndpoint lets an owner or admin rename the deployment. PB's
// own /api/settings is superuser-only, and an app owner is not a superuser.
func RegisterOrgNameEndpoint(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.POST("/api/org-info/name", func(re *core.RequestEvent) error {
			if re.Auth == nil {
				return re.UnauthorizedError("Sign in to rename the workspace.", nil)
			}
			if !isOrgAdmin(re.Auth) {
				return re.ForbiddenError("Only an owner or admin can rename the workspace.", nil)
			}
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
				return re.BadRequestError("Invalid request body.", nil)
			}
			if err := setOrgName(re.App, body.Name); err != nil {
				return re.BadRequestError(err.Error(), nil)
			}
			return re.JSON(http.StatusOK, map[string]string{"name": re.App.Settings().Meta.AppName})
		})
		return e.Next()
	})
}
