package main

import (
	"errors"
	"fmt"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/internal/config"
	"tinycld.org/cli/internal/deviceflow"
	"tinycld.org/cli/internal/keychain"
	"tinycld.org/cli/output"
	"tinycld.org/cli/ui"
)

// cliScopes mirrors the scope set granted to the seeded tinycld-cli client:
// the CLI is first-party, so it asks for everything up front and the consent
// screen shows the full list.
var cliScopes = []string{
	"profile",
	"mail:read", "mail:send",
	"drive:read", "drive:write",
	"contacts:read", "contacts:write",
	"calendar:read", "calendar:write",
}

// contextTokenStore adapts the keychain to client.TokenStore for one context.
// A missing credential maps to ErrAuthExpired: from a command's point of view
// "never logged in" and "session expired" call for the same fix.
type contextTokenStore struct {
	store   keychain.Store
	account string
}

func (s contextTokenStore) Load() (client.TokenSet, error) {
	var tok client.TokenSet
	if err := keychain.GetJSON(s.store, s.account, &tok); err != nil {
		if errors.Is(err, keychain.ErrNotFound) {
			return client.TokenSet{}, client.ErrAuthExpired
		}
		return client.TokenSet{}, err
	}
	return tok, nil
}

func (s contextTokenStore) Save(tok client.TokenSet) error {
	return keychain.SetJSON(s.store, s.account, tok)
}

// apiClient resolves the active context and builds an authenticated client
// for it.
func (d *deps) apiClient() (*client.Client, string, error) {
	cfg, err := d.loadConfig()
	if err != nil {
		return nil, "", err
	}
	name, ctx, err := cfg.Resolve(d.ctxFlag)
	if err != nil {
		return nil, "", err
	}
	store := contextTokenStore{store: d.store(), account: name}
	return client.New(ctx.Origin, store, d.httpClient), name, nil
}

type userinfoResponse struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Username string `json:"preferred_username"`
}

func newAuthCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Log in and out of TinyCld servers",
	}
	cmd.AddCommand(newAuthLoginCmd(d), newAuthLogoutCmd(d), newAuthStatusCmd(d))
	return cmd
}

func newAuthLoginCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "login <host>",
		Short: "Authenticate with a TinyCld server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			origin, err := config.NormalizeOrigin(args[0])
			if err != nil {
				return err
			}
			flow := &deviceflow.Flow{
				Origin:   origin,
				ClientID: client.DefaultClientID,
				HTTP:     d.httpClient,
				Sleep:    d.sleep,
			}
			da, err := flow.Start(cmd.Context(), cliScopes)
			if err != nil {
				return err
			}

			// The code and URL are required to complete the flow, so they
			// print regardless of --quiet.
			host := config.ContextName(origin)
			fmt.Fprintf(cmd.OutOrStdout(), "! First copy your one-time code: %s\n", da.UserCode)
			if d.isTTY && !d.yes {
				ui.WaitEnter(d.out, d.yes, cmd.InOrStdin(), cmd.OutOrStdout(),
					fmt.Sprintf("Press Enter to open %s in your browser...", host))
				if err := browser.OpenURL(da.VerificationURIComplete); err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Could not open a browser — visit %s\n", da.VerificationURIComplete)
				}
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Approve this device at: %s\n", da.VerificationURIComplete)
			}

			tok, err := flow.Poll(cmd.Context(), da)
			if err != nil {
				return err
			}

			store := contextTokenStore{store: d.store(), account: host}
			if err := store.Save(tok); err != nil {
				return fmt.Errorf("saving credentials: %w", err)
			}

			var user userinfoResponse
			c := client.New(origin, store, d.httpClient)
			if err := c.GetJSON(cmd.Context(), "/oauth/userinfo", &user); err != nil {
				return fmt.Errorf("fetching identity: %w", err)
			}

			cfg, err := d.loadConfig()
			if err != nil {
				return err
			}
			cfg.Contexts[host] = config.Context{Origin: origin, User: user.Email}
			cfg.Current = host
			if err := cfg.Save(d.configDir); err != nil {
				return err
			}

			d.out.Info(cmd.OutOrStdout(), "✓ Authenticated as %s", user.Email)
			d.out.Info(cmd.OutOrStdout(), "✓ Token saved to keychain")
			return nil
		},
	}
}

type authStatus struct {
	Context string `json:"context"`
	Origin  string `json:"origin"`
	Email   string `json:"email"`
	Scopes  string `json:"scopes"`
}

func newAuthStatusCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the logged-in user for the current context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, name, err := d.apiClient()
			if err != nil {
				return err
			}
			var user userinfoResponse
			if err := c.GetJSON(cmd.Context(), "/oauth/userinfo", &user); err != nil {
				return err
			}
			tok, err := c.Token()
			if err != nil {
				return err
			}
			status := authStatus{Context: name, Origin: c.Origin(), Email: user.Email, Scopes: tok.Scope}
			if d.out.Format == output.JSON {
				return d.out.Write(cmd.OutOrStdout(), nil, nil, status)
			}
			return d.out.Write(cmd.OutOrStdout(),
				[]string{"CONTEXT", "ORIGIN", "USER", "SCOPES"},
				[][]string{{status.Context, status.Origin, status.Email, status.Scopes}},
				status)
		},
	}
}

func newAuthLogoutCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke this device's access and forget its credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := d.loadConfig()
			if err != nil {
				return err
			}
			name, ctx, err := cfg.Resolve(d.ctxFlag)
			if err != nil {
				return err
			}

			store := contextTokenStore{store: d.store(), account: name}
			tok, loadErr := store.Load()
			if loadErr == nil && tok.RefreshToken != "" {
				// Best-effort server-side revocation: the local credential is
				// cleared regardless, so a network failure only leaves a grant
				// the user can still revoke from Settings → Connected apps.
				if err := revokeToken(d, ctx.Origin, tok.RefreshToken); err != nil {
					d.out.Info(cmd.ErrOrStderr(), "warning: could not revoke on the server: %v", err)
				}
			}
			if err := d.store().Delete(name); err != nil {
				return fmt.Errorf("clearing stored credentials: %w", err)
			}
			d.out.Info(cmd.OutOrStdout(), "✓ Logged out of %s", name)
			return nil
		},
	}
}

func revokeToken(d *deps, origin, token string) error {
	resp, err := d.httpClient.PostForm(origin+"/oauth/revoke",
		map[string][]string{"token": {token}})
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}
