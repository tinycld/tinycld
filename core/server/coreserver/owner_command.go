package coreserver

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

// generatedPasswordBytes is the entropy behind an auto-generated owner
// password: 24 random bytes, base64url-encoded to a 32-character secret. Well
// past anything brute-forceable, and safe to paste through a shell or a JSON
// body without escaping.
const generatedPasswordBytes = 24

// GenerateOwnerPassword returns a URL-safe random password. Exported so a
// caller that must know the password before invoking this command (the
// hosting router returns it to the operator) generates it the same way.
func GenerateOwnerPassword() (string, error) {
	b := make([]byte, generatedPasswordBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewCreateOwnerCommand builds the `create-owner` subcommand: mint the app's
// first operator against an existing pb_data, then exit.
//
// WHY THIS EXISTS. A single-tenant deployment gets its first accounts from the
// setup wizard, whose routes RegisterSetupBootstrap binds — in the HOST
// composition only. A hosted org has no wizard: the hosting router
// provisions it by building an artifact and booting a tenant, and the tenant
// composition never binds those routes. So a freshly provisioned org served
// fine but had zero users and nobody could log in.
//
// It creates BOTH identities the wizard does, with the same credentials:
//
//   - a `_superusers` record — the PocketBase admin behind /_/, which also
//     keeps PB's installer satisfied and backs the sharelink signing key;
//   - a `users` record with role=owner — the identity the APP authenticates
//     against and the /admin console runs as.
//
// Creating only the first is the trap this command exists to prevent: PB's own
// `superuser upsert` writes `_superusers` alone, so the app login still fails.
//
// Idempotent: provisioning may be retried, and a retry must not fail because
// one or both records already exist.
func NewCreateOwnerCommand(app *pocketbase.PocketBase) *cobra.Command {
	var password, passwordHash, name, orgName string

	cmd := &cobra.Command{
		Use:   "create-owner <email>",
		Short: "Create this deployment's first operator (superuser + owner account), then exit",
		Long: "Mints the two identities a deployment needs to be usable: a PocketBase " +
			"_superusers record (the /_/ admin) and a `users` record with role=owner " +
			"(the app login). The /setup wizard creates these on first run; a " +
			"deployment provisioned WITHOUT the wizard — a scripted install, a restored " +
			"backup, an automated provisioner — runs this instead, or it would serve " +
			"correctly with nobody able to log in. Pair with PB's --dir flag pointing at " +
			"the data directory. Without --password a random one is generated and " +
			"printed. Pass --password-hash instead of --password to mint both identities " +
			"from a bcrypt hash computed elsewhere; nothing is printed but the " +
			"confirmation line. --name sets the owner's display name. --org-name sets the " +
			"workspace name (shown at sign-in and in invite emails) and marks it as " +
			"chosen, so the setup wizard shows it instead of asking; it is applied on " +
			"every run. Re-running for an existing email leaves the accounts unchanged.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			email := strings.TrimSpace(args[0])
			if email == "" {
				return fmt.Errorf("create-owner: email is required")
			}
			if password != "" && passwordHash != "" {
				return fmt.Errorf("create-owner: pass --password or --password-hash, not both")
			}
			// Validate before touching either record: a bad hash written into
			// the superuser succeeds (raw password fields skip validation when
			// Plain is empty), and only the users side would then refuse it —
			// leaving a corrupt, permanent superuser behind.
			if passwordHash != "" && !IsBcryptHash(passwordHash) {
				return fmt.Errorf("create-owner: --password-hash must be a bcrypt hash")
			}
			// Same reason: refuse a bad name before any record exists.
			if orgName != "" {
				if _, err := normalizeOrgName(orgName); err != nil {
					return fmt.Errorf("create-owner: --org-name: %w", err)
				}
			}

			if password == "" && passwordHash == "" {
				generated, err := GenerateOwnerPassword()
				if err != nil {
					return fmt.Errorf("create-owner: %w", err)
				}
				password = generated
			}

			// Bootstrap opens the DB and applies SYSTEM migrations; the app's
			// own collections (`users`) come from the JS
			// migrations RunAppMigrations applies. In the router's flow the
			// tenant has already booted and run them, so this is a no-op —
			// but running against a fresh pb_data must work rather than fail
			// on a missing collection.
			if err := app.Bootstrap(); err != nil {
				return fmt.Errorf("create-owner: bootstrap: %w", err)
			}
			if err := app.RunAppMigrations(); err != nil {
				return fmt.Errorf("create-owner: run migrations: %w", err)
			}

			created, err := createOperatorIdentities(app, email, name, password, passwordHash)
			if err != nil {
				return fmt.Errorf("create-owner: %w", err)
			}
			if orgName != "" {
				if err := setOrgName(app, orgName); err != nil {
					return fmt.Errorf("create-owner: set workspace name: %w", err)
				}
				// Like the wizard row itself, a convenience: the name is saved
				// either way, and the owner can confirm it in the wizard.
				if err := markOrgNameSeeded(app); err != nil {
					srvLog.Warn("create-owner: could not record the workspace name in the setup wizard", "err", err)
				}
			}

			if !created {
				cmd.Printf("owner: %s\nunchanged: account already exists (password not modified)\n", email)
				return nil
			}
			// A hash run must never print a password: the caller already holds
			// the plaintext (it produced the hash) and printing it here would
			// be the exact process-boundary crossing this flag exists to avoid.
			if passwordHash != "" {
				cmd.Printf("owner: %s\n", email)
				return nil
			}
			// Report the password ONLY when this run actually set it. On a
			// no-op re-run the records already exist with their original
			// secret, and printing the freshly generated one would hand the
			// operator a password that does not work.
			cmd.Printf("owner: %s\npassword: %s\n", email, password)
			return nil
		},
	}

	cmd.Flags().StringVar(&password, "password", "",
		"password for both identities; omit to generate a random one")
	cmd.Flags().StringVar(&passwordHash, "password-hash", "",
		"bcrypt hash to use for both identities instead of a password")
	cmd.Flags().StringVar(&name, "name", "",
		"display name for the owner account (default: the email local-part)")
	cmd.Flags().StringVar(&orgName, "org-name", "",
		"workspace name; the setup wizard shows it as already chosen")
	return cmd
}

// createOperatorIdentities creates the _superusers record and the app owner,
// skipping whichever already exists. The two are created independently so a
// retry after a partial failure completes the missing half rather than
// erroring on the half that succeeded.
//
// Exactly one of password/passwordHash is set (the caller enforces that);
// whichever is present is what both identities are minted from.
//
// On every successful run (whether it created anything or not), it starts the
// setup wizard if no wizard row exists. This allows a deployment that was
// provisioned without the wizard (no `setup.wizard` row) to opt in via a
// create-owner re-run. The no-op when the row exists keeps wizard progress
// intact across re-runs.
//
// Reports whether it created anything, so the caller knows if the
// password/hash was actually applied — on a full no-op the existing records
// keep their original secret.
func createOperatorIdentities(app core.App, email, name, password, passwordHash string) (created bool, err error) {
	if existing, _ := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, email); existing == nil {
		superusers, ferr := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
		if ferr != nil {
			return created, fmt.Errorf("find superusers collection: %w", ferr)
		}
		su := core.NewRecord(superusers)
		su.SetEmail(email)
		if passwordHash != "" {
			su.SetRaw(core.FieldNamePassword, &core.PasswordFieldValue{Hash: passwordHash})
			su.RefreshTokenKey()
		} else {
			su.SetPassword(password)
		}
		su.SetVerified(true)
		if serr := app.Save(su); serr != nil {
			return created, fmt.Errorf("create superuser: %w", serr)
		}
		created = true
	}

	if existing, _ := app.FindAuthRecordByEmail("users", email); existing == nil {
		if passwordHash != "" {
			if _, cerr := CreateOwnerAccountWithHash(app, email, name, passwordHash); cerr != nil {
				return created, fmt.Errorf("create owner account: %w", cerr)
			}
		} else if _, cerr := CreateOwnerAccountNamed(app, email, name, password); cerr != nil {
			return created, fmt.Errorf("create owner account: %w", cerr)
		}
		created = true
	}

	// The wizard is a convenience; a deployment whose owner exists but whose
	// wizard row failed to write must still be provisioned. On re-run of an
	// existing deployment, this starts the wizard if it was never begun.
	if werr := MarkSetupWizardStarted(app); werr != nil {
		srvLog.Warn("create-owner: could not start the setup wizard", "err", werr)
	}
	return created, nil
}
