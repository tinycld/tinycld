package quota

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Setting keys read by SettingsLimits.
const (
	userLimitKey = "storage_limit_bytes"
	orgLimitKey  = "org_storage_limit_bytes"
)

// SettingsLimits resolves ceilings from the `settings` collection — the
// single-org deployment's source of truth. The collection has no access rules,
// so only a superuser can change them.
//
// Under multi-org the ORG ceiling must NOT come from here: each tenant has its
// own superusers, so an org could raise the plan limit you sold it. The router
// supplies that value instead; see FixedLimits.
func SettingsLimits(app core.App) Limits {
	return Limits{
		PerUser: settingInt(app, userLimitKey),
		PerOrg:  settingInt(app, orgLimitKey),
	}
}

// FixedLimits returns a LimitsFunc over values resolved once at boot — the
// tenant case, where the org ceiling arrives in the runtime config the router
// materialized and is not readable or writable from the org's own DB.
//
// The per-user ceiling still comes from settings, since that IS the org's own
// policy to set within whatever total it was allotted.
func FixedLimits(perOrg int64) LimitsFunc {
	return func(app core.App) Limits {
		return Limits{
			PerUser: settingInt(app, userLimitKey),
			PerOrg:  perOrg,
		}
	}
}

// settingInt reads one integer setting, treating absent/unparseable as 0
// (unlimited). A quota that fails open on a missing row is the right default:
// the operator has not set a limit.
func settingInt(app core.App, key string) int64 {
	var result struct {
		Value int64 `db:"val"`
	}
	err := app.DB().NewQuery(`
		SELECT COALESCE(CAST(value AS INTEGER), 0) AS val
		FROM settings
		WHERE app = 'core' AND key = {:key}
		LIMIT 1
	`).Bind(dbx.Params{"key": key}).One(&result)
	if err != nil {
		return 0
	}
	return result.Value
}
