import { useOrgInfo } from './use-org-info'

/**
 * System-settings key namespaces this deployment does not administer.
 *
 * A deployment run by a hosting provider does not own its own Sentry project,
 * web-push keypair or mail provider account — the provider does, for every
 * deployment it runs. Those values never reach this deployment's database, so
 * the screens that would edit them cannot work here and are hidden.
 *
 * The server reports the namespaces (never the values) on /api/org-info, which
 * is already fetched before login on web and native alike. Empty on a standalone
 * deployment, where every setting is the operator's own.
 */
export function useManagedSettingPrefixes(): string[] {
    return useOrgInfo().managedSettings
}

/**
 * Whether `keyPrefix` names settings administered elsewhere.
 *
 * Pass the dotted namespace the screen edits (`'mail.'`, `'vapid.'`). A screen
 * with no namespace is never managed, which keeps every existing package
 * unaffected — a panel must opt in by declaring what it writes.
 */
export function useIsSettingManaged(keyPrefix: string | undefined | null): boolean {
    const managed = useManagedSettingPrefixes()
    return isManagedPrefix(managed, keyPrefix)
}

/**
 * The matching rule, React-free so it can be unit-tested and reused by the help
 * filter without a hook.
 *
 * An empty or absent prefix never matches. Treating it as "matches everything"
 * would hide every screen the moment one namespace was managed — the opposite
 * of what a panel that declared nothing intended.
 */
export function isManagedPrefix(
    managedPrefixes: readonly string[],
    keyPrefix: string | undefined | null
): boolean {
    if (!keyPrefix) return false
    return managedPrefixes.some(prefix => prefix !== '' && keyPrefix.startsWith(prefix))
}
