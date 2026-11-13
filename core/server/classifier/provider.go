package classifier

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/appstate"
)

const openRouterURL = "https://openrouter.ai/api/alpha/decisions"

// Response is the provider's HTTP answer, parsed by parseResponse.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// Provider sends one Decisions payload. It returns an error only when the
// request may not have reached the provider; every HTTP answer, error codes
// included, is a Response.
type Provider interface {
	Decide(ctx context.Context, apiKey string, payload []byte) (Response, error)
}

type OpenRouter struct {
	URL    string
	Client *http.Client
}

func NewOpenRouter() *OpenRouter {
	return &OpenRouter{URL: openRouterURL, Client: &http.Client{Timeout: 30 * time.Second}}
}

func (o *OpenRouter) Decide(ctx context.Context, apiKey string, payload []byte) (Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.URL, bytes.NewReader(payload))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := o.Client.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Response{}, err
	}
	return Response{Status: resp.StatusCode, Header: resp.Header, Body: body}, nil
}

var (
	defaultMu       sync.RWMutex
	defaultProvider = func() Provider { return NewOpenRouter() }
	providerKey     = appstate.NewKey[Provider]("classifier.provider")
)

// SetDefaultProvider replaces the provider every app starts with. Only the
// classifierfake build tag calls it, so dev and e2e builds never reach
// OpenRouter.
func SetDefaultProvider(f func() Provider) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultProvider = f
}

// SetProvider overrides the provider for one app (unit tests).
func SetProvider(app core.App, p Provider) { providerKey.Set(app, p) }

func providerFor(app core.App) Provider {
	return providerKey.GetOrInit(app, func() Provider {
		defaultMu.RLock()
		defer defaultMu.RUnlock()
		return defaultProvider()
	})
}
