package maildomains

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/mrz1836/postmark"
)

// ErrAmbiguousServer means the account has several servers and none was
// named. Deliberately an error rather than a heuristic pick: guessing wrong
// sends the deployment's mail from the wrong Postmark server, which looks like
// working mail until someone reads the headers.
var ErrAmbiguousServer = errors.New("maildomains: account has multiple Postmark servers; set mail.postmark_server_name")

// PostmarkServers is the slice of the Postmark client this resolver needs, so
// tests do not need a live account.
type PostmarkServers interface {
	GetServers(ctx context.Context, count, offset int64, name string) (postmark.ServersList, error)
}

// serverListLimit bounds the listing. Postmark accounts hold tens of servers at
// most, and a deployment with more than this needs an explicit name anyway.
const serverListLimit = 100

// TokenResolver supplies the SERVER token, deriving it from the ACCOUNT token
// when one was not configured explicitly.
//
// Requiring an operator to paste both keys is redundant input — GetServers is
// an account-token call and returns each server's APITokens — and it admits a
// failure mode nothing checks for: a server token belonging to a DIFFERENT
// server than the account being managed yields a deployment where sending
// works and domain verification silently does not, with no error naming the
// mismatch. Deriving one from the other makes that unrepresentable.
//
// # Construct this ONLY from a router/supervisor composition, never inside a tenant
//
// NewTokenResolver requires the account token in-process to call GetServers.
// On a hosting deployment that token authenticates every domain on the
// account, not just this org's, so a tenant process holding it is a
// cross-tenant credential leak — the exact leak hosting/syscfg's
// TenantSyscfg filter exists to close (see hosting/cmd/serve-router/hooks.go's
// resolveServerToken, which is the one legitimate caller today). Do not wire
// this from coreserver's wireMailDomains or any other code path that runs
// inside a tenant: that composition only ever legitimately holds the SERVER
// token (syscfg's "mail.postmark_server_token", already filtered/derived
// upstream), never the account token, and reading
// syscfg.Get("mail.postmark_account_token") tenant-side will simply return
// empty on a hosted deployment — which looks like a bug to debug rather than
// the boundary working as intended.
type TokenResolver struct {
	accountToken    string
	configuredToken string
	serverName      string
	client          PostmarkServers

	mu     sync.Mutex
	cached string
}

// NewTokenResolver builds a resolver. configuredServerToken, when non-empty,
// short-circuits derivation entirely.
//
// Call this only from a router/supervisor composition that legitimately holds
// the account token — see the "Construct this ONLY..." note on TokenResolver
// above.
func NewTokenResolver(accountToken, configuredServerToken, serverName string, client PostmarkServers) *TokenResolver {
	return &TokenResolver{
		accountToken:    strings.TrimSpace(accountToken),
		configuredToken: strings.TrimSpace(configuredServerToken),
		serverName:      strings.TrimSpace(serverName),
		client:          client,
	}
}

// ServerToken returns the server token, resolving and caching it on first need.
//
// The cache lives for the life of the resolver and there is NO invalidation
// path, deliberately: no caller in this ecosystem is positioned to detect that
// the cached token went stale. The only composition that derives a token
// (hosting's router) ships it to tenants and never calls Postmark with it, so
// it never sees the auth failure a rotation would produce. An operator who
// rotates the server token inside Postmark must therefore restart the router,
// or change an input the caller's own cache keys on (hosting's resolverCache
// keys on the account token and server name). A resolver-level Invalidate()
// existed here for a while with no caller at all, which read as if that
// recovery were wired when it was not — a dead method is worse than an absent
// one. If a composition that USES the derived token ever appears, reintroduce
// invalidation together with that call site, not before it.
func (r *TokenResolver) ServerToken(ctx context.Context) (string, error) {
	if r.configuredToken != "" {
		return r.configuredToken, nil
	}
	if r.accountToken == "" {
		return "", ErrNotConfigured
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cached != "" {
		return r.cached, nil
	}

	list, err := r.client.GetServers(ctx, serverListLimit, 0, "")
	if err != nil {
		return "", fmt.Errorf("list postmark servers: %w", err)
	}
	token, err := selectServerToken(list.Servers, r.serverName)
	if err != nil {
		return "", err
	}
	r.cached = token
	return token, nil
}

// selectServerToken picks the server and returns its first API token.
func selectServerToken(servers []postmark.Server, name string) (string, error) {
	if name != "" {
		for _, s := range servers {
			if strings.EqualFold(s.Name, name) {
				return firstToken(s)
			}
		}
		return "", fmt.Errorf("no postmark server named %q on this account", name)
	}
	switch len(servers) {
	case 0:
		return "", errors.New("maildomains: postmark account has no servers")
	case 1:
		return firstToken(servers[0])
	default:
		return "", ErrAmbiguousServer
	}
}

func firstToken(s postmark.Server) (string, error) {
	if len(s.APITokens) == 0 {
		return "", fmt.Errorf("postmark server %q has no API tokens", s.Name)
	}
	return s.APITokens[0], nil
}
