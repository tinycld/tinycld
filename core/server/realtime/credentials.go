package realtime

import (
	"net/http"
	"strings"
)

// A browser cannot set headers on a WebSocket upgrade, so a realtime client
// sends its credential as a WebSocket subprotocol instead of in the URL: a
// URL lands in the request log and in every proxy's access log, a header
// does not. The client offers Protocol plus one credential protocol; the
// server answers with Protocol, which the browser requires to be one it
// offered.
const (
	// Protocol is the subprotocol every realtime client offers.
	Protocol = "tinycld.realtime"
	// AuthProtocolPrefix carries the PocketBase auth token.
	AuthProtocolPrefix = "tinycld.auth."
	// ShareProtocolPrefix carries an anonymous share-session token.
	ShareProtocolPrefix = "tinycld.share."
)

// connectCredentials is what a connecting client presented.
type connectCredentials struct {
	authToken    string
	shareSession string
}

// takeCredentials reads the credentials from the subprotocol header. Clients
// released before the header existed send them as ?token= / ?share_session=;
// those still work, and are removed from the request URL so the request log
// does not record them.
func takeCredentials(r *http.Request) connectCredentials {
	var creds connectCredentials
	for _, header := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, protocol := range strings.Split(header, ",") {
			protocol = strings.TrimSpace(protocol)
			if token, ok := strings.CutPrefix(protocol, AuthProtocolPrefix); ok {
				creds.authToken = token
			}
			if token, ok := strings.CutPrefix(protocol, ShareProtocolPrefix); ok {
				creds.shareSession = token
			}
		}
	}

	query := r.URL.Query()
	if !query.Has("token") && !query.Has("share_session") {
		return creds
	}
	if creds.authToken == "" {
		creds.authToken = query.Get("token")
	}
	if creds.shareSession == "" {
		creds.shareSession = query.Get("share_session")
	}
	query.Del("token")
	query.Del("share_session")
	r.URL.RawQuery = query.Encode()
	return creds
}
