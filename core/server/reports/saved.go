package reports

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// maxSpecBytes bounds a saved spec. It is the size of a draft run's whole
// body, so every spec the builder can run, it can save. The migration's json
// maxSize is the same number.
const maxSpecBytes = maxRunBody

// serverOwned are the reports fields only the server writes. A client value
// for any of them is replaced before the save.
var serverOwned = []string{"source", "published_by", "broken", "shipped_key", "shipped_revision"}

// isActiveAdmin reports a users record whose role is owner or admin and that
// is not disabled: the reader the rules call an admin.
func isActiveAdmin(user *core.Record) bool {
	if user == nil || user.GetBool("disabled") {
		return false
	}
	role := user.GetString("role")
	return role == "owner" || role == "admin"
}

func fieldError(at, code, message string) map[string]validation.Error {
	return map[string]validation.Error{at: validation.NewError(code, message)}
}

const adminOnly = "Only an admin can publish or unpublish a report."

func onReportCreate(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	for _, f := range serverOwned {
		e.Record.Set(f, nil)
	}
	if err := prepareSpec(e, nil); err != nil {
		return err
	}
	if e.Record.GetBool("published") {
		if !isActiveAdmin(e.Auth) {
			return e.ForbiddenError(adminOnly, fieldError("published", "admin_only", adminOnly))
		}
		e.Record.Set("published_by", e.Auth.Id)
	}
	return e.Next()
}

func onReportUpdate(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	original := e.Record.Original()
	if e.Record.GetString("owner") != original.GetString("owner") {
		return e.BadRequestError("A report's owner does not change.",
			fieldError("owner", "read_only", "A report's owner does not change."))
	}
	for _, f := range serverOwned {
		e.Record.Set(f, original.Get(f))
	}
	if err := prepareSpec(e, original); err != nil {
		return err
	}
	published := e.Record.GetBool("published")
	if published != original.GetBool("published") {
		if !isActiveAdmin(e.Auth) {
			return e.ForbiddenError(adminOnly, fieldError("published", "admin_only", adminOnly))
		}
		by := ""
		if published {
			by = e.Auth.Id
		}
		e.Record.Set("published_by", by)
	}
	return e.Next()
}

// prepareSpec reads the posted spec as a run reads it and stores it as Load
// returns it: upgraded to the current version, aliases resolved. That is how
// a spec is "saved upgraded on its next save". source becomes the spec's
// base source. A new spec, or a changed one, must pass Validate, and then
// the report is no longer broken. An update that leaves the spec as it was
// is not checked, valid or not, so a broken report can still be renamed.
func prepareSpec(e *core.RecordRequestEvent, original *core.Record) error {
	raw := specBytes(e.Record)
	if len(raw) > maxSpecBytes {
		msg := fmt.Sprintf("A report spec has at most %d bytes.", maxSpecBytes)
		return e.BadRequestError(msg, fieldError("spec", "too_large", msg))
	}
	spec, stored, err := loadStored(raw)
	if err != nil {
		if original != nil && bytes.Equal(raw, specBytes(original)) {
			return nil
		}
		return specRefusal(e, err)
	}
	if original == nil || !sameSpec(specBytes(original), stored) {
		if err := Validate(spec); err != nil {
			return specRefusal(e, err)
		}
		e.Record.Set("broken", nil)
	}
	e.Record.Set("spec", types.JSONRaw(stored))
	e.Record.Set("source", spec.Source)
	return nil
}

// loadStored reads a spec as a run reads it and returns it with the bytes
// that are stored for it: upgraded, aliases resolved. A client save and a
// shipped report both store this form.
func loadStored(raw []byte) (*Spec, []byte, error) {
	spec, _, err := Load(raw)
	if err != nil {
		return nil, nil, err
	}
	stored, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, err
	}
	return spec, stored, nil
}

// specBytes is a report's stored or posted spec.
func specBytes(r *core.Record) []byte {
	raw, _ := r.Get("spec").(types.JSONRaw)
	return raw
}

// sameSpec reports whether the stored spec, loaded, is the loaded spec
// about to be stored.
func sameSpec(raw, stored []byte) bool {
	_, before, err := loadStored(raw)
	return err == nil && bytes.Equal(before, stored)
}

// specRefusal answers a spec that does not load or validate exactly as the
// run endpoint answers it: 400 with each invalid part, 422 naming each
// unknown source, link, field or column, keyed by its spec path.
func specRefusal(e *core.RecordRequestEvent, err error) error {
	var specErr *SpecError
	if errors.As(err, &specErr) {
		status, message := specStatus(specErr)
		return e.Error(status, message, specErr.Problems)
	}
	log.ErrorContext(e.Request.Context(), "report spec check failed", "err", err)
	return e.InternalServerError("The report could not be saved.", nil)
}
