/// <reference path="../pb_data/types.d.ts" />
// Put the login guard `@request.auth.id != ""` on every path of every core
// rule that reads @request.auth.
//
// For a request with no login, PocketBase resolves @request.auth.id to NULL
// and rewrites `x = NULL` as `(x = '' OR x IS NULL)`. So `user =
// @request.auth.id` matches every row whose `user` is empty. The rules below
// are safe today only because each column they test is required (or is the
// row id), and a role test such as `@request.auth.role = "admin"` is false
// for an empty role. A later migration that makes a column optional would
// open the rule to anonymous callers with no change to the rule itself: that
// is how group grants opened a package's comment_mentions branch. The guard
// removes the dependency on the column. For a caller with a login the guard
// is always true, so no signed-in behaviour changes.
//
// rlstest.RequireAuthGuardOnAccessRules fails the build on any rule that
// reads @request.auth on a path without the guard.
//
// Rules are restated as literals: `before` is what the earlier migrations
// left, and down restores it exactly.
const GUARD = '@request.auth.id != ""'
const OWN_ROW = 'user = @request.auth.id'
const ORG_ADMIN = '(@request.auth.role = "admin" || @request.auth.role = "owner")'

const USERS_READ_BEFORE =
    '((@request.auth.id != "" && @request.auth.role != "guest") || id = @request.auth.id) || ' +
    '@request.auth.id != "" && @request.auth.disabled != true && (@request.auth.role = "owner" || @request.auth.role = "admin")'
const USERS_READ_AFTER =
    '(@request.auth.id != "" && @request.auth.role != "guest") || (@request.auth.id != "" && id = @request.auth.id) || ' +
    '@request.auth.id != "" && @request.auth.disabled != true && (@request.auth.role = "owner" || @request.auth.role = "admin")'

const RULES_READ_BEFORE = 'owner = @request.auth.id || (scope = "org" && @request.auth.id != "" && @request.auth.role != "guest")'
const RULES_UPDATE_BEFORE =
    '(scope = "personal" && owner = @request.auth.id && (@request.body.scope:isset = false || @request.body.scope = "personal") && ' +
    '(@request.body.owner:isset = false || @request.body.owner = @request.auth.id)) || ' +
    `(scope = "org" && ${ORG_ADMIN})`
const RULES_DELETE_BEFORE = `(scope = "personal" && owner = @request.auth.id) || (scope = "org" && ${ORG_ADMIN})`
const RULE_RUNS_READ_BEFORE = `rule.owner = @request.auth.id || (rule.scope = "org" && ${ORG_ADMIN})`

const guarded = rule => `${GUARD} && (${rule})`

// { collection: { kind: [before, after] } }
const CHANGES = {
    users: {
        listRule: [USERS_READ_BEFORE, USERS_READ_AFTER],
        viewRule: [USERS_READ_BEFORE, USERS_READ_AFTER],
        deleteRule: ['id = @request.auth.id', `${GUARD} && id = @request.auth.id`],
    },
    label_assignments: {
        listRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
        viewRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
        createRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
        updateRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
        deleteRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
    },
    push_subscriptions: {
        listRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
        viewRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
        updateRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
        deleteRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
    },
    user_preferences: {
        listRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
        viewRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
        updateRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
        deleteRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
    },
    notifications: {
        listRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
        viewRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
        updateRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
        deleteRule: [OWN_ROW, `${GUARD} && ${OWN_ROW}`],
    },
    rules: {
        listRule: [RULES_READ_BEFORE, guarded(RULES_READ_BEFORE)],
        viewRule: [RULES_READ_BEFORE, guarded(RULES_READ_BEFORE)],
        updateRule: [RULES_UPDATE_BEFORE, guarded(RULES_UPDATE_BEFORE)],
        deleteRule: [RULES_DELETE_BEFORE, guarded(RULES_DELETE_BEFORE)],
    },
    rule_runs: {
        listRule: [RULE_RUNS_READ_BEFORE, guarded(RULE_RUNS_READ_BEFORE)],
        viewRule: [RULE_RUNS_READ_BEFORE, guarded(RULE_RUNS_READ_BEFORE)],
    },
}

function setRules(app, pick) {
    for (const [name, rules] of Object.entries(CHANGES)) {
        const col = app.findCollectionByNameOrId(name)
        for (const [kind, pair] of Object.entries(rules)) {
            col[kind] = pick(pair)
        }
        app.save(col)
    }
}

migrate(
    app => setRules(app, ([, after]) => after),
    app => setRules(app, ([before]) => before)
)
