package webforms

import (
	"database/sql"
	"errors"
	"fmt"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
	"tinycld.org/core/embedpolicy"
	"tinycld.org/core/forms"
)

const (
	WebFormHostKey      = "core:web_form"
	publicationsName    = "form_publications"
	submissionsName     = "form_submissions"
	submissionFilesName = "form_submission_files"
	tokenLength         = 32
	defaultRetention    = 90
	defaultMaxUploadMb  = 25
	defaultConfirmation = "Thank you. We received your submission."
)

func bindPublicationHooks(app core.App) {
	app.OnRecordCreate(publicationsName).BindFunc(func(e *core.RecordEvent) error {
		e.Record.Set("token", security.RandomString(tokenLength))
		if err := preparePublication(e.App, e.Record); err != nil {
			return err
		}
		return e.Next()
	})
	app.OnRecordUpdate(publicationsName).BindFunc(func(e *core.RecordEvent) error {
		// The server owns the token: empty asks for a new one; anything else
		// must be the stored value. Read the stored row, not Original(): a
		// record saved earlier in the same process keeps its pre-create
		// original.
		stored, err := e.App.FindRecordById(publicationsName, e.Record.Id)
		if err != nil {
			return err
		}
		switch e.Record.GetString("token") {
		case "":
			e.Record.Set("token", security.RandomString(tokenLength))
		case stored.GetString("token"):
		default:
			return validation.Errors{"token": validation.NewError("validation_token_server_owned",
				"The server sets the link token. Leave token out to keep it, or send an empty token to make a new one.")}
		}
		if err := preparePublication(e.App, e.Record); err != nil {
			return err
		}
		return e.Next()
	})
}

// preparePublication applies defaults, normalizes the origins and checks the
// form is a web form.
func preparePublication(app core.App, rec *core.Record) error {
	if rec.GetInt("retention_days") == 0 {
		rec.Set("retention_days", defaultRetention)
	}
	if rec.GetInt("max_upload_mb") == 0 {
		rec.Set("max_upload_mb", defaultMaxUploadMb)
	}
	if rec.GetString("confirmation") == "" {
		rec.Set("confirmation", defaultConfirmation)
	}
	origins, err := embedpolicy.ParseOrigins(rec.GetString("allowed_domains"))
	if err != nil {
		return validation.Errors{"allowed_domains": validation.NewError("validation_invalid_origin", err.Error())}
	}
	rec.Set("allowed_domains", origins)
	form, err := app.FindRecordById("forms", rec.GetString("form"))
	if err != nil {
		return validation.Errors{"form": validation.NewError("validation_not_web_form", "No form has this id.")}
	}
	if form.GetString("host") != WebFormHostKey {
		return validation.Errors{"form": validation.NewError("validation_not_web_form",
			fmt.Sprintf("Only a web form (host %s) can be published. This form's host is %q.", WebFormHostKey, form.GetString("host")))}
	}
	return nil
}

func findPublication(app core.App, token string) (*core.Record, error) {
	if token == "" {
		return nil, sql.ErrNoRows
	}
	return app.FindFirstRecordByData(publicationsName, "token", token)
}

// livePublication returns the publication, its form and definition when the
// token names an enabled publication of an active web form; nil records
// otherwise, with a nil error for "not live".
func livePublication(app core.App, token string) (*core.Record, *core.Record, *forms.Definition, error) {
	if forms.HostFor(app, WebFormHostKey) == nil {
		log.Error("web form host is not registered; refusing to serve web forms")
		return nil, nil, nil, nil
	}
	pub, err := findPublication(app, token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if !pub.GetBool("enabled") {
		return nil, nil, nil, nil
	}
	form, err := app.FindRecordById("forms", pub.GetString("form"))
	if err != nil || form.GetBool("archived") || form.GetString("host") != WebFormHostKey {
		return nil, nil, nil, nil
	}
	def, err := forms.ParseDefinition([]byte(form.GetString("definition")))
	if err != nil {
		return nil, nil, nil, err
	}
	return pub, form, def, nil
}
