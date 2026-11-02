package webforms

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/embedpolicy"
)

const embedRoutePrefix = "/p/forms/"

type publicForm struct {
	Name         string          `json:"name"`
	Confirmation string          `json:"confirmation"`
	Slug         string          `json:"slug"`
	Definition   json.RawMessage `json:"definition"`
	RequirePow   bool            `json:"requirePow"`
	MaxUploadMb  int             `json:"maxUploadMb"`
	Stamp        string          `json:"stamp"`
	MinDelayMs   int             `json:"minDelayMs"`
}

func handlePublicForm(app core.App, re *core.RequestEvent) error {
	pub, form, _, err := livePublication(app, re.Request.PathValue("token"))
	if err != nil {
		return re.InternalServerError("", err)
	}
	if pub == nil {
		return re.NotFoundError("This form is not available", nil)
	}
	key, err := subkey(app, stampLabel)
	if err != nil {
		return re.InternalServerError("", err)
	}
	return re.JSON(http.StatusOK, publicForm{
		Name:         form.GetString("name"),
		Confirmation: pub.GetString("confirmation"),
		Slug:         form.GetString("slug"),
		Definition:   json.RawMessage(form.GetString("definition")),
		RequirePow:   pub.GetBool("require_pow"),
		MaxUploadMb:  pub.GetInt("max_upload_mb"),
		Stamp:        issueStamp(key, pub.Id, time.Now()),
		MinDelayMs:   int(stampMinAge / time.Millisecond),
	})
}

func handleChallenge(app core.App, re *core.RequestEvent) error {
	pub, _, _, err := livePublication(app, re.Request.PathValue("token"))
	if err != nil {
		return re.InternalServerError("", err)
	}
	if pub == nil || !pub.GetBool("require_pow") {
		return re.NotFoundError("", nil)
	}
	key, err := subkey(app, powLabel)
	if err != nil {
		return re.InternalServerError("", err)
	}
	c, err := newChallenge(key, pub.Id, time.Now())
	if err != nil {
		return re.InternalServerError("", err)
	}
	return re.JSON(http.StatusOK, c)
}

// embedToken returns the token of a public form page, or "" for any other
// path. Strict: one segment exactly, so /p/forms/<token>/x gets no grant.
func embedToken(r *http.Request) string {
	if r == nil || r.URL == nil || !strings.HasPrefix(r.URL.Path, embedRoutePrefix) {
		return ""
	}
	token := strings.TrimPrefix(r.URL.Path, embedRoutePrefix)
	if token == "" || strings.Contains(token, "/") {
		return ""
	}
	return token
}

// registerEmbedPolicy frames a known publication's page at its allowed
// origins, or anywhere when the list is empty. A disabled publication still
// frames, so the page can say "not available" inside the client's site
// instead of a browser error. An unknown token is not claimed.
func registerEmbedPolicy(app core.App) {
	embedpolicy.Register(app, func(r *http.Request) []string {
		pub, err := findPublication(app, embedToken(r))
		if err != nil {
			return nil
		}
		origins := strings.Fields(pub.GetString("allowed_domains"))
		if len(origins) == 0 {
			return []string{"*"}
		}
		return origins
	})
}
