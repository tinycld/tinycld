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
func NewTokenResolver(accountToken, configuredServerToken, serverName string, client PostmarkServers) *TokenResolver {
	return &TokenResolver{
		accountToken:    strings.TrimSpace(accountToken),
		configuredToken: strings.TrimSpace(configuredServerToken),
		serverName:      strings.TrimSpace(serverName),
		client:          client,
	}
}

// ServerToken returns the server token, resolving and caching it on first need.
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

// Invalidate drops the cached token so the next call re-resolves. Called after
// a Postmark auth failure, which is what a rotated token looks like from here.
func (r *TokenResolver) Invalidate() {
	r.mu.Lock()
	r.cached = ""
	r.mu.Unlock()
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
