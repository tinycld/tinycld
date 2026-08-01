package coreserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// DeployChannel is the tenant side of the per-org control socket (design D1):
// an HTTP client over the ROUTER-bound unix socket the router handed us via
// --control-socket. The socket's 0700 tenant-uid directory is the
// authentication — a tenant can only ever reach its own channel, so every
// call carries this org's identity by construction.
//
// The surface mirrors controlplane.Deployer.Handler's versioned protocol:
// state/resolve/versions are the hosted Packages UI's metadata pre-flight
// (the confined tenant has no npm/node/git of its own), build is the warm-up
// (the org keeps serving while a novel set pays its build minutes), and
// deploy is the D6 step-2 proposal after which the tenant waits to be killed.
type DeployChannel struct {
	// short-lived metadata calls (state, resolve, versions, deploy).
	client *http.Client
	// build waits for a full artifact build — minutes for a novel set.
	buildClient *http.Client

	// Per-spec cache for Versions, mirroring the 60s cache pkgbuild keeps for
	// the single-tenant versions handler. Hosted lost that caching by moving
	// discovery across the socket — and the router THROTTLES ctl reads
	// per-org, so the Packages screen re-mounting during a login flow (one
	// /v1/versions per registry row per mount) burned the org's read budget
	// and rows silently degraded to "no available versions". Entries die with
	// the process (every deploy respawns the tenant), so staleness is bounded
	// by both the TTL and the tenant's own lifetime.
	verMu    sync.Mutex
	verCache map[string]cachedCtlVersions
}

type cachedCtlVersions struct {
	val     ctlVersions
	fetched time.Time
}

// ctlVersionsTTL matches pkgbuild's versionCacheTTL — the same discovery the
// single-tenant handler serves from its own 60s cache.
const ctlVersionsTTL = 60 * time.Second

// ErrCtlRateLimited marks a control-socket 429 — the router's per-org
// throttles (the 30s deploy-proposal floor, the read-endpoint bucket).
// Transient by construction; callers retry rather than fail.
var ErrCtlRateLimited = errors.New("control socket rate-limited")

// NewDeployChannel dials socketPath for every request.
func NewDeployChannel(socketPath string) *DeployChannel {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	return &DeployChannel{
		// Resolve shells npm pack on the router; versions npm view. Generous
		// but bounded.
		client:      &http.Client{Transport: transport, Timeout: 5 * time.Minute},
		buildClient: &http.Client{Transport: transport, Timeout: 90 * time.Minute},
	}
}

// ctlState is GET /v1/state's payload: the org row's authoritative current
// lockfile and recipe hash.
type ctlState struct {
	Lockfile   map[string]string `json:"lockfile"`
	RecipeHash string            `json:"recipeHash"`
}

// ctlBuildResult is POST /v1/build's payload. Migrations is the committed
// artifact's pb_migrations basenames — the "new set" side of the tenant's
// down/up delta (D6).
type ctlBuildResult struct {
	RecipeHash string   `json:"recipeHash"`
	Cached     bool     `json:"cached"`
	Migrations []string `json:"migrations"`
}

// ctlResolvedSpec is POST /v1/resolve's payload (builder.ResolvedSpec's wire
// shape — one fetch spec's trusted-side identity + contribution facts).
type ctlResolvedSpec struct {
	Slug         string            `json:"slug"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Integrity    string            `json:"integrity"`
	PeerVersions map[string]string `json:"peerVersions"`
	Manifest     json.RawMessage   `json:"manifest"`
	Migrations   []string          `json:"migrations"`
}

// ctlVersions is POST /v1/versions's payload.
type ctlVersions struct {
	Source   string   `json:"source"`
	Versions []string `json:"versions"`
	Error    string   `json:"error"`
}

func (c *DeployChannel) State(ctx context.Context) (ctlState, error) {
	var out ctlState
	err := c.call(ctx, c.client, http.MethodGet, "/v1/state", nil, &out)
	return out, err
}

func (c *DeployChannel) Resolve(ctx context.Context, spec string) (ctlResolvedSpec, error) {
	var out ctlResolvedSpec
	err := c.call(ctx, c.client, http.MethodPost, "/v1/resolve", map[string]any{"spec": spec}, &out)
	return out, err
}

func (c *DeployChannel) Versions(ctx context.Context, spec string) (ctlVersions, error) {
	c.verMu.Lock()
	if e, ok := c.verCache[spec]; ok && time.Since(e.fetched) < ctlVersionsTTL {
		c.verMu.Unlock()
		return e.val, nil
	}
	c.verMu.Unlock()

	var out ctlVersions
	err := c.call(ctx, c.client, http.MethodPost, "/v1/versions", map[string]any{"spec": spec}, &out)
	if err != nil {
		return out, err
	}
	// Cache successes only — a throttled/failed discovery must retry, not pin
	// an empty list for a minute. Specs come from pkg_registry rows (bounded),
	// so no size cap is needed.
	c.verMu.Lock()
	if c.verCache == nil {
		c.verCache = map[string]cachedCtlVersions{}
	}
	c.verCache[spec] = cachedCtlVersions{val: out, fetched: time.Now()}
	c.verMu.Unlock()
	return out, nil
}

func (c *DeployChannel) Build(ctx context.Context, lock map[string]string) (ctlBuildResult, error) {
	var out ctlBuildResult
	err := c.call(ctx, c.buildClient, http.MethodPost, "/v1/build", map[string]any{"lockfile": lock}, &out)
	return out, err
}

// Deploy proposes the next lockfile (D6 step 2's "sends {next lockfile} and
// waits to be killed"). A nil error means the router recorded the proposal
// and repointed the org — the eviction follows asynchronously.
func (c *DeployChannel) Deploy(ctx context.Context, lock map[string]string, jobID string) error {
	return c.call(ctx, c.client, http.MethodPost, "/v1/deploy",
		map[string]any{"lockfile": lock, "jobId": jobID}, &struct{}{})
}

// call performs one JSON round trip. Non-2xx responses surface the handler's
// {"error": ...} body as the error text.
func (c *DeployChannel) call(ctx context.Context, client *http.Client, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	// The host portion is ignored by the unix-socket dialer but must parse.
	req, err := http.NewRequestWithContext(ctx, method, "http://control"+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("control socket %s: %w", path, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("control socket %s: read response: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error string `json:"error"`
		}
		wrap := func(msg string) error {
			// 429 is typed so callers can RETRY instead of failing the whole
			// operation: the router floors accepted deploy proposals per org
			// (30s), and a legitimate back-to-back sequence — an admin
			// downgrading right after an upgrade landed — trips it.
			if resp.StatusCode == http.StatusTooManyRequests {
				return fmt.Errorf("%w: %s", ErrCtlRateLimited, msg)
			}
			return fmt.Errorf("%s", msg)
		}
		if json.Unmarshal(payload, &e) == nil && e.Error != "" {
			return wrap(e.Error)
		}
		return wrap(fmt.Sprintf("control socket %s: HTTP %d", path, resp.StatusCode))
	}
	if out != nil {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("control socket %s: parse response: %w", path, err)
		}
	}
	return nil
}
