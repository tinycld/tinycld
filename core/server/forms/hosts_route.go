package forms

import (
	"net/http"
	"sort"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// CustomKinds lists the kinds a custom field may use, sorted: every kind the
// validator knows, less "custom" and "section", which are not field kinds.
func CustomKinds() []string {
	out := make([]string, 0, len(operatorsByKind))
	for k := range operatorsByKind {
		if k == "custom" || k == "section" {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type builtinView struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	Required bool   `json:"required"`
}

type hostView struct {
	Key      string        `json:"key"`
	Label    string        `json:"label"`
	Kinds    []string      `json:"kinds"`
	Builtins []builtinView `json:"builtins"`
}

func hostViewOf(h Host) hostView {
	kinds := h.Kinds
	if len(kinds) == 0 {
		kinds = CustomKinds()
	}
	builtins := make([]builtinView, 0, len(h.Builtins))
	for _, b := range h.Builtins {
		builtins = append(builtins, builtinView{Key: b.Key, Label: b.Label, Kind: b.Kind, Required: b.Required})
	}
	return hostView{Key: h.Key, Label: h.Label, Kinds: kinds, Builtins: builtins}
}

// BindHostsRoute serves GET /api/forms/hosts: the installed form hosts, with
// the field kinds each allows and its built-in fields, so a client that has
// no generated host list (the CLI) can write a valid definition. Any
// signed-in user who is not a guest may read it, as with the forms list.
func BindHostsRoute(app core.App, defs *Defs) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/api/forms/hosts", func(re *core.RequestEvent) error {
			if re.Auth.GetString("role") == "guest" {
				return re.ForbiddenError("A guest cannot read the form hosts.", nil)
			}
			views := make([]hostView, 0, len(defs.Hosts))
			for _, h := range defs.Hosts {
				views = append(views, hostViewOf(h))
			}
			return re.JSON(http.StatusOK, map[string]any{"hosts": views})
		}).Bind(apis.RequireAuth())
		return se.Next()
	})
}
