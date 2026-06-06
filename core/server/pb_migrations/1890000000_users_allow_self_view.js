/// <reference path="../pb_data/types.d.ts" />
// SECURITY: amend users.list/viewRule to allow self-access.
//
// 1870000000_exclude_guests_from_org_rls tightened users.list/viewRule to
// "non-guest member of a shared org." That correctly blocks roster
// enumeration by a guest, but it also blocks a guest from reading their
// OWN users record — the rule requires a non-guest user_org row on the
// same relation-path prefix, and a pure guest only has guest rows.
//
// PocketBase's auth-refresh path (and any client.collection('users').
// getOne(authId)) needs a guest to be able to load themselves. Without
// that, pb.authStore.save(token, record) is followed by an automatic
// authRefresh that 404s, dropping the client back to anon — the share
// route then renders as anon despite a successful OTP verify.
//
// Carve-out: `|| id = @request.auth.id`. Symmetric with the carve-out
// 1870000000 already added on user_org (`|| user = @request.auth.id`)
// and orgs (`|| user_org_via_org.user ?= @request.auth.id`). The
// roster-leak property still holds: the guest's list-collection query
// matches only their own row, not other members' rows (verified in
// guest_rls_test.go's TestGuestRLS_Users_GuestCannotSeeMemberEmail).
//
// The down-migration restores the EXACT 1870000000 rule string.
migrate(
    app => {
        const usersRule =
            '@request.auth.id != "" && (' +
            '(user_org_via_user.org.user_org_via_org.user ?= @request.auth.id && ' +
            'user_org_via_user.org.user_org_via_org.role ?!= "guest")' +
            ' || id = @request.auth.id' +
            ')'
        const users = app.findCollectionByNameOrId('users')
        users.listRule = usersRule
        users.viewRule = usersRule
        app.save(users)
    },
    app => {
        const usersPrior =
            '@request.auth.id != "" && ' +
            'user_org_via_user.org.user_org_via_org.user ?= @request.auth.id && ' +
            'user_org_via_user.org.user_org_via_org.role ?!= "guest"'
        const users = app.findCollectionByNameOrId('users')
        users.listRule = usersPrior
        users.viewRule = usersPrior
        app.save(users)
    }
)
