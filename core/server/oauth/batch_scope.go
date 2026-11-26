package oauth

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// batchPath is PocketBase's batch route: several record writes of any
// collections in one transaction. No scope can name it, because a scope names
// collections. So a token's batch is authorized by its operations: every one
// must be a record write its scopes allow, as if it were sent alone.
const batchPath = "/api/batch"

// defaultBatchBodySize is PocketBase's own limit when the setting is not set
// (apis/batch.go).
const defaultBatchBodySize = 128 << 20

func isBatchRoute(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == batchPath
}

// batchOperations is the batch body. It has the shape and the json tag of
// PocketBase's own form (apis.batchRequestsForm), and authorizeBatch reads it
// with the same BindBody: what is checked here is what the handler runs.
type batchOperations struct {
	Requests []*core.InternalRequest `json:"requests"`
}

// authorizeBatch refuses a token's batch unless every operation in it is a
// record write that granted allows. It fails closed: a body it cannot read as
// PocketBase will, an operation that is not a record route, and a collection
// no package registered are all refused.
func authorizeBatch(re *core.RequestEvent, granted []string) error {
	// A multipart batch carries files, and its operations in a form field; only
	// the JSON form is read here, so only the JSON form is let through.
	if !strings.HasPrefix(re.Request.Header.Get("Content-Type"), "application/json") {
		return re.ForbiddenError("A batch from an API token must be JSON", nil)
	}
	limit := re.App.Settings().Batch.MaxBodySize
	if limit <= 0 {
		limit = defaultBatchBodySize
	}
	// The body is read here, before the handler applies its limit. A stated
	// length bounds the read; a body with no stated length is not read.
	if re.Request.ContentLength < 0 {
		return re.ForbiddenError("A batch from an API token must state its length", nil)
	}
	if re.Request.ContentLength > limit {
		return router.NewApiError(http.StatusRequestEntityTooLarge, "The batch is too large", nil)
	}
	var body batchOperations
	if err := re.BindBody(&body); err != nil {
		return re.BadRequestError("Failed to read the submitted batch data.", err)
	}
	// Each operation is checked as its record route would be: the route's
	// scope, then the expand, filter and sort of its query.
	for i, op := range body.Requests {
		rule := ScopeRule(nil)
		var path string
		var query url.Values
		if op != nil {
			path, _, _ = strings.Cut(op.URL, "?")
			// The handler builds the operation's request with
			// http.NewRequest, so its query is the one url.Parse reads (a
			// fragment is not part of it). A URL that does not parse is no
			// record write.
			parsed, err := url.Parse(op.URL)
			if err == nil && isRecordWrite(op.Method, path) {
				rule = ScopeForRoute(op.Method, path)
				query = parsed.Query()
			}
		}
		if len(rule) == 0 || rule.isExempt() {
			return re.ForbiddenError(fmt.Sprintf("Operation %d of the batch is not available to API tokens", i+1), nil)
		}
		if !rule.SatisfiedBy(granted) {
			return re.ForbiddenError(fmt.Sprintf("Operation %d of the batch requires the %s scope", i+1, rule.describe()), nil)
		}
		if err := checkRelatedReads(re.App, path, query, granted); err != nil {
			return re.ForbiddenError(fmt.Sprintf("Operation %d of the batch: %s", i+1, err), nil)
		}
	}
	return nil
}

// isRecordWrite reports whether method and path are one of the record writes a
// batch runs (apis.ValidBatchActions): a create or an upsert of
// /api/collections/{c}/records, an update or a delete of
// /api/collections/{c}/records/{id}. The method must be uppercase: the
// handler uppercases a method and runs it, but this check refuses a method in
// another case, so such a batch fails closed.
func isRecordWrite(method, path string) bool {
	const prefix = "/api/collections/"
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	switch method {
	case http.MethodPost, http.MethodPut:
		return len(parts) == 2 && parts[1] == "records"
	case http.MethodPatch, http.MethodDelete:
		return len(parts) == 3 && parts[1] == "records"
	}
	return false
}
