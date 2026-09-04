// The share token for the current session, if any.
//
// A share-link visitor is UNAUTHENTICATED — there is no auth record, no
// membership row and nothing for `@request.auth.id` to match. What authorizes
// them is a token, carried on every request as `X-Share-Token` and validated
// inside the collection access rules (boards' pb-migrations/1980000003 is the
// first user of this).
//
// It lives in a module-level variable rather than a store because the two
// readers cannot use a hook: `pb.beforeSend` runs inside the PocketBase client,
// and pbtsdb's `subscribeOptions` is called at subscribe time, on reconnect and
// whenever a collection's subscriber count rises from zero. Both need the value
// that is true AT THAT MOMENT, which is also why the collection factory is
// handed a getter and not a captured string — see setShareToken's note on
// sign-in.

let shareToken: string | null = null

/**
 * Set (or clear) the token every subsequent request carries.
 *
 * Pass `null` on sign-in. This is not tidiness: once a visitor redeems a link
 * they hold a real membership, and the OTHER half of the rule disjunct — the
 * membership branch — is what authorizes them from then on. Continuing to send
 * a token would keep presenting a credential that is no longer the basis for
 * their access, and would survive its own revocation.
 */
export function setShareToken(token: string | null) {
    shareToken = token || null
}

export function getShareToken(): string | null {
    return shareToken
}

/**
 * The header a share-link request carries, or undefined when there is no token.
 *
 * PocketBase snakecases header names before a rule sees them, so this reaches
 * the rule engine as `@request.headers.x_share_token` — identical on the REST
 * path and through a realtime subscription's per-topic options, which is what
 * lets ONE rule serve both transports.
 */
export function shareTokenHeaders(): { 'X-Share-Token': string } | undefined {
    return shareToken ? { 'X-Share-Token': shareToken } : undefined
}
