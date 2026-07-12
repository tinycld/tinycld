/// <reference path="../pb_data/types.d.ts" />
// push_subscriptions and user_preferences originally shipped with a createRule
// of just `@request.auth.id != ""`, which let any authed user create a row
// owned by *another* user (the `user` relation was never pinned to the caller).
// For push_subscriptions that enabled a cross-user push hijack: an attacker
// could POST a subscription row with `user = <victim>` and an attacker-owned
// endpoint, and push.SendToUser would then deliver the victim's content to the
// attacker. Pin both createRules so the row's `user` must equal the authed id.
migrate(
    app => {
        const pinnedRule = '@request.auth.id != "" && user = @request.auth.id'

        const pushSubs = app.findCollectionByNameOrId('push_subscriptions')
        pushSubs.createRule = pinnedRule
        app.save(pushSubs)

        const userPrefs = app.findCollectionByNameOrId('user_preferences')
        userPrefs.createRule = pinnedRule
        app.save(userPrefs)
    },
    app => {
        const originalRule = '@request.auth.id != ""'

        const pushSubs = app.findCollectionByNameOrId('push_subscriptions')
        pushSubs.createRule = originalRule
        app.save(pushSubs)

        const userPrefs = app.findCollectionByNameOrId('user_preferences')
        userPrefs.createRule = originalRule
        app.save(userPrefs)
    }
)
