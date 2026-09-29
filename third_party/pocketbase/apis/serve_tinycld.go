package apis

// Fork-only: BuildServeMux, and the base router construction that it shares
// with Serve. See third_party/pocketbase/FORK.md.

import (
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/pocketbase/pocketbase/ui"
)

// errServeMuxNotInitialized is returned when the OnServe hook completes without
// building the router mux (typically a handler forgot to call e.Next()).
var errServeMuxNotInitialized = errors.New("the OnServe hook did not initialize the router mux. Did you forget to call the ServeEvent.Next() method?")

// buildBaseRouter constructs the app's base router: NewRouter plus the default
// CORS binding and the admin UI static route. It is shared by Serve and
// BuildServeMux so both produce an identical base router.
//
// The body is upstream Serve's router setup, moved here verbatim. When an
// upstream merge conflicts in Serve at the buildBaseRouter call, carry the
// upstream change into this function.
func buildBaseRouter(app core.App, config ServeConfig) (*router.Router[*core.RequestEvent], error) {
	if len(config.AllowedOrigins) == 0 {
		config.AllowedOrigins = []string{"*"}
	}

	pbRouter, err := NewRouter(app)
	if err != nil {
		return nil, err
	}

	pbRouter.Bind(CORS(CORSConfig{
		AllowOrigins: config.AllowedOrigins,
		AllowMethods: []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete},
	}))

	// @todo consider moving in base
	if ui.DistDirFS != nil {
		pbRouter.GET("/_/{path...}", Static(ui.DistDirFS, false)).
			BindFunc(func(e *core.RequestEvent) error {
				if !e.App.IsDev() &&
					// exclude root path
					e.Request.PathValue(StaticWildcardParam) != "" &&
					e.Response.Header().Get("Cache-Control") == "" {
					e.Response.Header().Set("Cache-Control", "max-age=1209600, stale-while-revalidate=86400")
				}

				if e.Response.Header().Get("Content-Security-Policy") == "" {
					e.Response.Header().Set("Content-Security-Policy", defaultCSP)
				}

				return e.Next()
			}).
			Bind(Gzip())
	}

	return pbRouter, nil
}

// BuildServeMux builds and returns the app's HTTP handler (mux) without starting
// a server or listener. It constructs the base router, fires the OnServe hook so
// plugins can bind their routes, and returns the built mux.
//
// This is the reusable core of Serve for embedders that manage their own
// http.Server (e.g. multi-app routers): call BuildServeMux per app and dispatch
// to the returned handlers. The app must already be bootstrapped.
//
// Note: OnServe is triggered with a nil ServeEvent.Server, CertManager, and
// Listener, since no server is started here. OnServe handlers that dereference
// those fields must nil-check; the built-in plugins bind via e.Router and are
// unaffected.
//
// Do not also call Serve on the same app: OnServe would fire a second time and
// re-register routes (BuildMux would then fail) and restart cron. Use
// BuildServeMux OR Serve for a given app, not both.
func BuildServeMux(app core.App, config ServeConfig) (http.Handler, error) {
	pbRouter, err := buildBaseRouter(app, config)
	if err != nil {
		return nil, err
	}

	var handler http.Handler
	serveEvent := new(core.ServeEvent)
	serveEvent.App = app
	serveEvent.Router = pbRouter
	serveEvent.InstallerFunc = DefaultInstallerFunc // set for OnServe event parity; no installer is launched here

	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		h, err := e.Router.BuildMux()
		if err != nil {
			return err
		}
		handler = h
		return nil
	})
	if err != nil {
		return nil, err
	}
	if handler == nil {
		return nil, errServeMuxNotInitialized
	}

	return handler, nil
}
