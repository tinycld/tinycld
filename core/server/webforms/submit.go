package webforms

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"tinycld.org/core/forms"
	"tinycld.org/core/logging"
	"tinycld.org/core/ratelimit"
)

const (
	// The route allows the largest publication cap; each publication's own
	// cap is checked against the parsed parts.
	maxBodyBytes    = 101 << 20
	multipartMemory = 8 << 20
	maxClientID     = 64
	maxUserAgent    = 300
)

var (
	log        = logging.ForPackage("webforms")
	ipLimiter  = ratelimit.New(5, 10*time.Minute)
	pubLimiter = ratelimit.New(200, time.Hour)
	spentPow   = newPowLedger()
)

func resetLimitersForTesting() {
	ipLimiter.Reset()
	pubLimiter.Reset()
	spentPow = newPowLedger()
}

func bindSubmitRoute(app core.App, se *core.ServeEvent) {
	se.Router.POST("/api/forms/public/{token}/submit", func(re *core.RequestEvent) error {
		return handleSubmit(app, re)
	}).Bind(apis.BodyLimit(maxBodyBytes))
}

func fieldError(key, code, msg string) validation.Errors {
	return validation.Errors{key: validation.NewError(code, msg)}
}

// sameOrigin: the page posts from inside the iframe, so a real submit's
// Origin is this server. Another site posting from a visitor's browser is not.
// Behind a proxy that rewrites Host, X-Forwarded-Host counts only when the
// admin lists it as a trusted proxy header (passkey's rp.go does the same);
// otherwise a client could name any host it likes.
func sameOrigin(app core.App, r *http.Request) bool {
	u, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || u.Host == "" {
		return false
	}
	hosts := []string{r.Host}
	for _, h := range app.Settings().TrustedProxy.Headers {
		if strings.EqualFold(h, "X-Forwarded-Host") {
			hosts = append(hosts, r.Header.Get("X-Forwarded-Host"))
		}
	}
	for _, h := range hosts {
		if h != "" && strings.EqualFold(u.Host, h) {
			return true
		}
	}
	return false
}

func ok(re *core.RequestEvent) error {
	return re.JSON(http.StatusOK, map[string]bool{"ok": true})
}

// handleSubmit runs the checks cheapest first and stops at the first failure.
func handleSubmit(app core.App, re *core.RequestEvent) error {
	ctx := re.Request.Context()
	pub, form, def, err := livePublication(app, re.Request.PathValue("token"))
	if err != nil {
		return re.InternalServerError("", err)
	}
	if pub == nil {
		return re.NotFoundError("This form is not available", nil)
	}
	if !sameOrigin(app, re.Request) {
		return re.ForbiddenError("", nil)
	}
	if err := re.Request.ParseMultipartForm(multipartMemory); err != nil {
		return re.BadRequestError("The submission could not be read", nil)
	}
	r := re.Request
	if r.FormValue("_hp") != "" {
		log.InfoContext(ctx, "honeypot filled, submission dropped", "publication", pub.Id)
		return ok(re)
	}

	stampKey, err := subkey(app, stampLabel)
	if err != nil {
		return re.InternalServerError("", err)
	}
	switch err := checkStamp(stampKey, r.FormValue("stamp"), pub.Id, time.Now()); {
	case errors.Is(err, errTooFast):
		return re.BadRequestError("Please wait and try again", fieldError("stamp", "validation_too_fast", "Please wait and try again"))
	case err != nil:
		return re.BadRequestError("Please reload the form and try again", fieldError("stamp", "validation_invalid_stamp", "Please reload the form and try again"))
	}

	ip := re.RealIP()
	if !ipLimiter.AllowOrLog(pub.Id+"|"+ip, "webforms.submit.ip") || !pubLimiter.AllowOrLog(pub.Id, "webforms.submit.publication") {
		return re.TooManyRequestsError("Please wait and try again", nil)
	}

	if pub.GetBool("require_pow") {
		if err := checkPow(app, r.FormValue("pow"), pub.Id); err != nil {
			return re.BadRequestError("Please wait and try again", fieldError("pow", "validation_invalid_pow", "Please wait and try again"))
		}
	}

	uploads, ferrs := checkFiles(def, r.MultipartForm.File, pub.GetInt("max_upload_mb"))
	if ferrs != nil {
		return re.BadRequestError("The form has errors", ferrs)
	}

	clientID := r.FormValue("client_id")
	if clientID == "" || len(clientID) > maxClientID {
		return re.BadRequestError("The submission could not be read", nil)
	}
	_, err = app.FindFirstRecordByFilter(submissionsName, "publication = {:p} && client_id = {:c}",
		map[string]any{"p": pub.Id, "c": clientID})
	if err == nil {
		return ok(re)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return re.InternalServerError("", err)
	}

	values := map[string]any{}
	if err := json.Unmarshal([]byte(r.FormValue("values")), &values); err != nil {
		return re.BadRequestError("The submission could not be read", nil)
	}
	ipHash, err := hashIP(app, ip)
	if err != nil {
		return re.InternalServerError("", err)
	}
	if err := store(app, pub, form, def, values, uploads, clientID, ipHash, r.UserAgent()); err != nil {
		var verrs validation.Errors
		if errors.As(err, &verrs) {
			return re.BadRequestError("The form has errors", verrs)
		}
		return re.InternalServerError("", err)
	}
	return ok(re)
}

func checkPow(app core.App, encoded, pubID string) error {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return errPow
	}
	var s Solution
	if err := json.Unmarshal(raw, &s); err != nil {
		return errPow
	}
	key, err := subkey(app, powLabel)
	if err != nil {
		return err
	}
	return spentPow.verify(key, s, pubID, time.Now())
}

// store writes the submission and its files in one transaction. File-row ids
// are made first and written into values, so the host-record validation sees
// a required file field as filled; a single file field holds one id string.
// Client-sent values for file keys are dropped first, so a visitor cannot
// point a file field at another submission's file.
func store(app core.App, pub, form *core.Record, def *forms.Definition, values map[string]any,
	uploads map[string][]*multipart.FileHeader, clientID, ipHash, userAgent string) error {
	type pending struct {
		id, key string
		header  *multipart.FileHeader
	}
	var files []pending
	for _, f := range def.Fields {
		if f.Kind == "file" {
			delete(values, f.Key)
		}
	}
	for key, headers := range uploads {
		ids := make([]any, 0, len(headers))
		for _, h := range headers {
			id := core.GenerateDefaultRandomId()
			ids = append(ids, id)
			files = append(files, pending{id: id, key: key, header: h})
		}
		if def.Field(key).Multiple {
			values[key] = ids
		} else {
			values[key] = ids[0]
		}
	}
	if len(userAgent) > maxUserAgent {
		userAgent = userAgent[:maxUserAgent]
	}
	return app.RunInTransaction(func(tx core.App) error {
		subs, err := tx.FindCollectionByNameOrId(submissionsName)
		if err != nil {
			return err
		}
		sub := core.NewRecord(subs)
		sub.Set("form", form.Id)
		sub.Set("publication", pub.Id)
		sub.Set("values", values)
		sub.Set("client_id", clientID)
		sub.Set("ip_hash", ipHash)
		sub.Set("user_agent", userAgent)
		if err := tx.Save(sub); err != nil {
			return err
		}
		filesCol, err := tx.FindCollectionByNameOrId(submissionFilesName)
		if err != nil {
			return err
		}
		for _, p := range files {
			file, err := filesystem.NewFileFromMultipart(p.header)
			if err != nil {
				return err
			}
			rec := core.NewRecord(filesCol)
			rec.Id = p.id
			rec.Set("submission", sub.Id)
			rec.Set("field", p.key)
			rec.Set("file", file)
			rec.Set("size", p.header.Size)
			rec.Set("name", p.header.Filename)
			if err := tx.Save(rec); err != nil {
				return err
			}
		}
		return nil
	})
}
