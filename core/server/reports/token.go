package reports

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/oauth"
)

// refuseTokenDraft refuses a draft spec to an OAuth token: a token runs a
// saved report its user can open, by id. A draft is the report builder's
// preview, and its refusals name sources and fields, which the builder's own
// routes keep from a token.
func refuseTokenDraft(re *core.RequestEvent, spec json.RawMessage) error {
	if oauth.GrantedScopes(re) == nil || spec == nil {
		return nil
	}
	return re.ForbiddenError("An access token runs a saved report only: send reportId, not spec.", nil)
}

// closedToScopes names the collections a run read that the granted scopes of
// an OAuth token do not let it read through the records API, sorted. They are
// the collections the run's statements selected rows from (Result.collections)
// and those of each source a choice param reads its labels from, whether this
// run read a label or not.
//
// A collection that only a list rule reads is not one of them: the records
// API applies the same rule for a token and asks no scope for it.
func closedToScopes(app core.App, granted []string, spec *Spec, res *Result) []string {
	read := slices.Clone(res.collections)
	for _, def := range spec.Params {
		if def.OptionsFrom == nil {
			continue
		}
		if src := lookupSource(app, def.OptionsFrom.Source); src != nil {
			read = append(read, src.collections()...)
		}
	}
	slices.Sort(read)
	var closed []string
	for _, name := range slices.Compact(read) {
		if !oauth.CanReadCollection(granted, name) {
			closed = append(closed, name)
		}
	}
	return closed
}

// refuseTokenReads refuses the result of a run to an OAuth token whose scopes
// do not cover what the run read (closedToScopes). A session has no scope
// ceiling and always passes.
//
// A scope that opens the run and export routes says nothing about data, and a
// report reads the collections of any package: without this check a token of
// one package would read another package's rows. The user's own list rules
// filtered the rows before the check, as for a session.
//
// It runs after the run because only the run knows what its statements read.
// A run writes nothing, so a refused one leaves no trace.
func refuseTokenReads(re *core.RequestEvent, spec *Spec, res *Result) error {
	granted := oauth.GrantedScopes(re)
	if granted == nil {
		return nil
	}
	closed := closedToScopes(re.App, granted, spec, res)
	if len(closed) == 0 {
		return nil
	}
	return re.ForbiddenError("The access token's scopes do not cover what this report reads: "+strings.Join(closed, ", ")+".", nil)
}
