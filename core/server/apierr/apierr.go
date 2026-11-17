// Package apierr keeps the reason a request failed in the response. A record
// hook that refuses a save (a seat limit, a guard) returns an *ApiError with a
// message meant for the user; a handler that wraps every error in one generic
// message throws that reason away.
package apierr

import (
	"errors"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("apierr")

// Wrap returns err unchanged when it is (or wraps) an *router.ApiError. Any
// other error is logged and returned as status with the fallback message;
// PocketBase keeps validation field errors in the response data.
func Wrap(re *core.RequestEvent, status int, fallback string, err error) error {
	var apiErr *router.ApiError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	log.WarnContext(re.Request.Context(), fallback, "error", err, "path", re.Request.URL.Path)
	return router.NewApiError(status, fallback, err)
}
