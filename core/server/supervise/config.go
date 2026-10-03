package supervise

import (
	"errors"
	"strings"
	"time"
)

// defaultHTTPAddr is the plain-HTTP bind when HTTP_ADDR is unset: an
// unprivileged port that a reverse proxy or a Docker port mapping fronts.
const defaultHTTPAddr = "0.0.0.0:7090"

// The ports autocert mode holds: :443 for HTTPS, :80 for the ACME HTTP-01
// challenge and the redirect to HTTPS.
const (
	autocertHTTPSAddr = "0.0.0.0:443"
	autocertHTTPAddr  = "0.0.0.0:80"
)

// childRestartExitCode is what a server child too old for the control
// protocol exits with to ask for a restart (coreserver's restartExitCode).
const childRestartExitCode = 75

// portWant is one listener a child should get: its inherited-fd name and
// the address to bind.
type portWant struct {
	name, addr string
}

// config is what the supervisor reads from its environment once, at start.
type config struct {
	stateDir string
	// childArgs is the full argument list after the binary: serve, the
	// caller's directory flags, then the mode's domains and flags.
	childArgs []string
	// base is the main HTTP(S) listeners, before any package's ports.
	base []portWant
}

// readConfig applies the entrypoint's serve-mode rules. Autocert mode needs
// both AUTOCERT_ENABLED and PRIMARY_DOMAIN; with only the first, it serves
// plain HTTP, as the entrypoint did. Domain validation stays in the
// entrypoint, which runs before this.
func readConfig(args []string, getenv func(string) string) (config, error) {
	cfg := config{stateDir: strings.TrimSpace(getenv("TINYCLD_STATE_DIR"))}
	if cfg.stateDir == "" {
		return config{}, errors.New("TINYCLD_STATE_DIR is not set; the supervisor needs it to find the current build")
	}

	cfg.childArgs = append([]string{"serve"}, args...)
	primary := strings.TrimSpace(getenv("PRIMARY_DOMAIN"))
	autocert := autocertOn(getenv("AUTOCERT_ENABLED"))

	if autocert && primary != "" {
		cfg.childArgs = append(cfg.childArgs, primary)
		for _, d := range strings.Split(getenv("ADDITIONAL_DOMAINS"), ",") {
			if d = strings.TrimSpace(d); d != "" {
				cfg.childArgs = append(cfg.childArgs, d)
			}
		}
		// PocketBase serves TLS on the main listener only when --https is
		// set; with --http alone it would serve plain HTTP on :443.
		cfg.childArgs = append(cfg.childArgs, "--http="+autocertHTTPAddr, "--https="+autocertHTTPSAddr)
		cfg.base = []portWant{{ListenerHTTPS, autocertHTTPSAddr}, {ListenerHTTPRedirect, autocertHTTPAddr}}
		return cfg, nil
	}

	if autocert {
		log.Warn("AUTOCERT_ENABLED is set but PRIMARY_DOMAIN is empty; serving plain HTTP")
	}
	addr := strings.TrimSpace(getenv("HTTP_ADDR"))
	if addr == "" {
		addr = defaultHTTPAddr
	}
	cfg.childArgs = append(cfg.childArgs, "--http="+addr)
	cfg.base = []portWant{{ListenerHTTP, addr}}
	return cfg, nil
}

func autocertOn(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// options are the supervisor's time bounds. They are fields rather than
// constants so tests can shorten them.
type options struct {
	// readyTimeout is how long a new child has to send ready: the same 60 s
	// the entrypoint's health probe allowed a cold boot that runs
	// migrations and seeds packages.
	readyTimeout time.Duration
	// drainBound is how long a draining child has to exit before it is
	// killed: its own drain budget plus time to run its shutdown hooks.
	drainBound time.Duration
	// stopBound is how long a stopped child has after SIGTERM before
	// SIGKILL.
	stopBound time.Duration
}

func defaultOptions() options {
	return options{
		readyTimeout: 60 * time.Second,
		drainBound:   ChildDrainTimeout + 10*time.Second,
		stopBound:    10 * time.Second,
	}
}
