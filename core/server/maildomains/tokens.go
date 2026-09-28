package maildomains

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

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
	CreateServer(ctx context.Context, req postmark.ServerCreateRequest) (postmark.Server, error)
}

// serverListLimit bounds the listing. Postmark accounts hold tens of servers at
// most, and a deployment with more than this needs an explicit name anyway.
const serverListLimit = 100

// retryAfter is how long a failed derivation is remembered. Every send and
// every provider check asks for the token, so without it a wrong account
// token would call Postmark on each of them.
const retryAfter = 30 * time.Second

// TokenResolver supplies the SERVER token, deriving it from the ACCOUNT token
// when one was not configured explicitly.
//
// The account token is the one credential a person has to paste: an account
// can list its servers and create one, and each server carries its own API
// token. Asking for both was redundant input, and it admitted a failure mode
// nothing checks for: a server token belonging to a DIFFERENT server than the
// account being managed yields a deployment where sending works and domain
// verification silently does not, with no error naming the mismatch. Deriving
// one from the other makes that unrepresentable.
//
// Where it runs: in the process that legitimately holds the account token.
// On a standalone server that is the server itself. A composition that keeps
// the account token away from an org's process ships that org a configured
// server token instead, which short-circuits derivation, so the resolver
// there never sees an account token and never calls Postmark.
type TokenResolver struct {
	accountToken    string
	configuredToken string
	serverName      string
	createAs        string
	client          PostmarkServers

	mu        sync.Mutex
	cached    string
	lastErr   error
	lastErrAt time.Time
}

// NewTokenResolver builds a resolver. configuredServerToken, when non-empty,
// short-circuits derivation entirely. serverName picks a server by name when
// the account has several. createAs names the server to create when the
// account has none (or the named one is missing); empty means never create.
func NewTokenResolver(accountToken, configuredServerToken, serverName, createAs string, client PostmarkServers) *TokenResolver {
	return &TokenResolver{
		accountToken:    strings.TrimSpace(accountToken),
		configuredToken: strings.TrimSpace(configuredServerToken),
		serverName:      strings.TrimSpace(serverName),
		createAs:        strings.TrimSpace(createAs),
		client:          client,
	}
}

// ServerToken returns the server token, resolving and caching it on first need.
//
// A success is cached for the life of the resolver, with no invalidation
// path, deliberately: no caller is positioned to detect that the cached
// token went stale. An operator who rotates the server token inside Postmark
// must change an input the owner of this resolver keys on (the account token
// or the server name), which builds a fresh resolver. A failure is remembered
// for retryAfter so a bad token does not turn every send into a Postmark call.
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
	if r.lastErr != nil && time.Since(r.lastErrAt) < retryAfter {
		return "", r.lastErr
	}

	token, err := r.derive(ctx)
	if err != nil {
		r.lastErr, r.lastErrAt = err, time.Now()
		return "", err
	}
	r.cached, r.lastErr = token, nil
	return token, nil
}

func (r *TokenResolver) derive(ctx context.Context) (string, error) {
	list, err := r.client.GetServers(ctx, serverListLimit, 0, "")
	if err != nil {
		return "", fmt.Errorf("list postmark servers: %w", err)
	}
	token, err := selectServerToken(list.Servers, r.serverName)
	if err == nil {
		return token, nil
	}
	name := r.serverName
	if name == "" {
		name = r.createAs
	}
	if name == "" || !errors.Is(err, errNoServer) {
		return "", err
	}
	created, err := r.client.CreateServer(ctx, postmark.ServerCreateRequest{
		Name:         name,
		DeliveryType: "Live",
	})
	if err != nil {
		return "", fmt.Errorf("create postmark server %q: %w", name, err)
	}
	return firstToken(created)
}

// errNoServer marks the one selection failure that creating a server fixes:
// nothing to pick from. An ambiguous account is not that, and creating one
// more server there would only deepen the ambiguity.
var errNoServer = errors.New("maildomains: no matching postmark server")

// selectServerToken picks the server and returns its first API token.
func selectServerToken(servers []postmark.Server, name string) (string, error) {
	if name != "" {
		for _, s := range servers {
			if strings.EqualFold(s.Name, name) {
				return firstToken(s)
			}
		}
		return "", fmt.Errorf("%w: none named %q on this account", errNoServer, name)
	}
	switch len(servers) {
	case 0:
		return "", fmt.Errorf("%w: the account has no servers", errNoServer)
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

// The process-wide server-token source. Senders ask it for the Postmark
// server token instead of reading the setting themselves, so a deployment
// that stores only an account token still sends: the source derives the
// server token from it. The default knows nothing and reports not configured.
var (
	tokenMu     sync.RWMutex
	tokenSource = func(context.Context) (string, error) { return "", ErrNotConfigured }
)

// SetServerTokenSource installs the source that ServerToken consults.
func SetServerTokenSource(f func(ctx context.Context) (string, error)) {
	if f == nil {
		return
	}
	tokenMu.Lock()
	tokenSource = f
	tokenMu.Unlock()
}

// ServerToken returns the Postmark server token this deployment sends with:
// the configured one, else one derived from the account token.
func ServerToken(ctx context.Context) (string, error) {
	tokenMu.RLock()
	f := tokenSource
	tokenMu.RUnlock()
	return f(ctx)
}
