package apis

import (
	"io"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"
)

var ErrRequestEntityTooLarge = router.NewApiError(http.StatusRequestEntityTooLarge, "Request entity too large", nil)

const DefaultMaxBodySize int64 = 32 << 20 // @todo consider replacing with router.DefaultMaxMemory

const (
	DefaultBodyLimitMiddlewareId       = "pbBodyLimit"
	DefaultBodyLimitMiddlewarePriority = DefaultRateLimitMiddlewarePriority + 10
)

// BodyLimit returns a middleware handler that changes the default request body size limit.
//
// If limitBytes <= 0, no limit is applied.
//
// Otherwise, if the request body size exceeds the configured limitBytes,
// it sends 413 error response.
func BodyLimit(limitBytes int64) *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id:       DefaultBodyLimitMiddlewareId,
		Priority: DefaultBodyLimitMiddlewarePriority,
		Func: func(e *core.RequestEvent) error {
			err := applyBodyLimit(e, limitBytes)
			if err != nil {
				return err
			}

			return e.Next()
		},
	}
}

func dynamicCollectionBodyLimit(collectionPathParam string) *hook.Handler[*core.RequestEvent] {
	if collectionPathParam == "" {
		collectionPathParam = "collection"
	}

	return &hook.Handler[*core.RequestEvent]{
		Id:       DefaultBodyLimitMiddlewareId,
		Priority: DefaultBodyLimitMiddlewarePriority,
		Func: func(e *core.RequestEvent) error {
			collection, err := e.App.FindCachedCollectionByNameOrId(e.Request.PathValue(collectionPathParam))
			if err != nil {
				return e.NotFoundError("Missing or invalid collection context.", err)
			}

			limitBytes := DefaultMaxBodySize
			if !collection.IsView() {
				for _, f := range collection.Fields {
					if calc, ok := f.(core.MaxBodySizeCalculator); ok {
						limitBytes += calc.CalculateMaxBodySize()
					}
				}
			}

			err = applyBodyLimit(e, limitBytes)
			if err != nil {
				return err
			}

			return e.Next()
		},
	}
}

func applyBodyLimit(e *core.RequestEvent, limitBytes int64) error {
	// no limit
	if limitBytes <= 0 {
		return nil
	}

	// optimistically check the submitted request content length
	if e.Request.ContentLength > limitBytes {
		return ErrRequestEntityTooLarge
	}

	// replace the request body
	e.Request.Body = newMaxBytesReader(e.Request.Body, limitBytes)

	return nil
}

func newMaxBytesReader(body io.ReadCloser, limitBytes int64) *maxBytesReader {
	return &maxBytesReader{
		ReadCloser: body,
		limit:      limitBytes,
		remaining:  limitBytes,
	}
}

// maxBytesReader is very similar to the http.MaxBytesReader but support
// rereads and doesn't try to prematurely close the related response
// to allow consequent middlewares to operate correctly.
type maxBytesReader struct {
	io.ReadCloser
	limit     int64
	remaining int64
	stickyErr error
}

func (r *maxBytesReader) Read(b []byte) (int, error) {
	if r.stickyErr != nil {
		return 0, r.stickyErr
	}

	if len(b) == 0 {
		return 0, nil
	}

	// if possible no need to read the entire chunk since
	// remaining+1 is enough to determine whether it exceed the limit
	if int64(len(b))-1 > r.remaining {
		b = b[:r.remaining+1]
	}

	n, err := r.ReadCloser.Read(b)

	if int64(n) <= r.remaining {
		r.remaining -= int64(n)
		r.stickyErr = err
		return n, err
	}

	n = int(r.remaining)

	r.remaining = 0
	r.stickyErr = ErrRequestEntityTooLarge

	return n, r.stickyErr
}

// explicit casts to ensure that the main struct methods will be invoked
// (extra precautions in case of nested interface wrapping erasure)
// ---

func (r *maxBytesReader) Reread() {
	rereader, ok := r.ReadCloser.(router.Rereader)
	if ok {
		rereader.Reread()
		r.remaining = r.limit
		r.stickyErr = nil
	}
}

func (r *maxBytesReader) Close() error {
	return r.ReadCloser.Close()
}
