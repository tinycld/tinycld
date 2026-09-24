package rlstest

import "testing"

// Core's own rules, alone: every access path needs a login except the ones
// CorePublicPaths names. Packages run the same scan with their migrations
// applied on top.
func TestCoreMigrations_EveryAccessPathRequiresLogin(t *testing.T) {
	app := NewAssembledApp(t)
	RequireAuthGuardOnAccessRules(t, app, CorePublicPaths()...)
}
