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
	var password string

	cmd := &cobra.Command{
		Use:   "create-owner <email>",
		Short: "Create the org's first operator (superuser + owner account), then exit",
		Long: "Mints the two identities a hosted org needs to be usable: a PocketBase " +
			"_superusers record (the /_/ admin) and a `users` record with role=owner " +
			"(the app login). This is what the /setup wizard creates " +
			"on a single-tenant deployment; a hosted org has no wizard, so its router runs " +
			"this instead. Pair with PB's --dir flag pointing at the org's pb_data. " +
			"Without --password a random one is generated and printed. Re-running for an " +
			"existing email is a no-op.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			email := strings.TrimSpace(args[0])
			if email == "" {
				return fmt.Errorf("create-owner: email is required")
			}

			if password == "" {
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

			created, err := createOperatorIdentities(app, email, password)
			if err != nil {
				return fmt.Errorf("create-owner: %w", err)
			}

			// Report the password ONLY when this run actually set it. On a
			// no-op re-run the records already exist with their original
			// secret, and printing the freshly generated one would hand the
			// operator a password that does not work.
			if !created {
				cmd.Printf("owner: %s\nunchanged: account already exists (password not modified)\n", email)
				return nil
			}
			cmd.Printf("owner: %s\npassword: %s\n", email, password)
			return nil
		},
	}

	cmd.Flags().StringVar(&password, "password", "",
		"password for both identities; omit to generate a random one")
	return cmd
}

// createOperatorIdentities creates the _superusers record and the app owner,
// skipping whichever already exists. The two are created independently so a
// retry after a partial failure completes the missing half rather than
// erroring on the half that succeeded.
//
// Reports whether it created anything, so the caller knows if `password` was
// actually applied — on a full no-op the existing records keep their original
// secret.
func createOperatorIdentities(app core.App, email, password string) (created bool, err error) {
	if existing, _ := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, email); existing == nil {
		superusers, ferr := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
		if ferr != nil {
			return created, fmt.Errorf("find superusers collection: %w", ferr)
		}
		su := core.NewRecord(superusers)
		su.SetEmail(email)
		su.SetPassword(password)
		su.SetVerified(true)
		if serr := app.Save(su); serr != nil {
			return created, fmt.Errorf("create superuser: %w", serr)
		}
		created = true
	}

	if existing, _ := app.FindAuthRecordByEmail("users", email); existing != nil {
		return created, nil
	}
	if _, cerr := CreateOwnerAccount(app, email, password); cerr != nil {
		return created, fmt.Errorf("create owner account: %w", cerr)
	}
	return true, nil
}
