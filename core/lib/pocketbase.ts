import AsyncStorage from '@react-native-async-storage/async-storage'
import { QueryCache, QueryClient } from '@tanstack/react-query'
import { type MergedPackageSchema, tinycldConfig } from '@tinycld/app-generated/tinycld-config'
import { captureException } from '@tinycld/core/lib/errors'
import { buildPackageStores } from '@tinycld/core/lib/packages/derive-stores'
import type { Orgs, Schema, UserOrg, Users } from '@tinycld/core/types/pbSchema'
import { BasicIndex, createCollection, createReactProvider, setLogger } from 'pbtsdb'
import PocketBase, { AsyncAuthStore } from 'pocketbase'
import { Platform } from 'react-native'
import { PB_SERVER_ADDR } from './config'
import { createReachabilityTracker } from './server-reachability'
import { useConnectivityStore } from './stores/connectivity-store'
import type { UserSession } from './types'

export { eq } from '@tanstack/db'

// MergedSchema = core's Schema intersected with each linked package's schema.
// MergedPackageSchema is a PRECOMPUTED LITERAL intersection from the generated
// config (NOT derived from the config values) to avoid a circular type
// reference through coreStores. buildPackageStores wires stores from the config
// values at runtime.
type MergedSchema = Schema & MergedPackageSchema

if (Platform.OS !== 'web') {
    // Only polyfill EventSource on native — the browser has its own
    import('react-native-sse').then(mod => {
        global.EventSource = mod.default as unknown as typeof global.EventSource
    })
}

export { PB_SERVER_ADDR }

// Defer AsyncStorage access to avoid calling the native module during module evaluation,
// which crashes on React Native before the bridge is ready (AsyncStorage v3+)
const initialAuthPromise =
    typeof window !== 'undefined'
        ? new Promise<string | null>(resolve => {
              setTimeout(() => resolve(AsyncStorage.getItem('pb_auth')), 0)
          })
        : Promise.resolve(null)

const store = new AsyncAuthStore({
    save: async serialized => AsyncStorage.setItem('pb_auth', serialized),
    clear: async () => await AsyncStorage.removeItem('pb_auth'),
})

// AsyncAuthStore's own `initial` option runs through a private
// fire-and-forget _loadInitial that swallows errors, races consumers, and
// gives no readiness signal. Instead we hydrate explicitly: read storage,
// parse, call store.save(). authStoreReady resolves only after that
// completes, so anyone awaiting it sees a settled pb.authStore.
export const authStoreReady = initialAuthPromise.then(storedAuth => {
    if (!storedAuth) return
    try {
        const parsed = JSON.parse(storedAuth) as {
            token?: string
            record?: unknown
            model?: unknown
        }
        if (parsed.token) {
            store.save(parsed.token, (parsed.record ?? parsed.model) as never)
        }
    } catch {
        // Corrupt stored auth — leave the store empty and let the user
        // re-authenticate. Clearing storage avoids replaying the corruption
        // on next boot.
        AsyncStorage.removeItem('pb_auth')
    }
})

export const pb = new PocketBase(PB_SERVER_ADDR, store)

pb.autoCancellation(false)

// The server-unreachable signal (which drives the offline overlay) is
// derived from pb.send outcomes via a rolling sustained-failure tracker.
// See server-reachability.ts for the rationale — in short, a single blip,
// an aborted request, or a failed auth-refresh must NOT flip the badge;
// only several genuine network failures in quick succession do.
const reachability = createReachabilityTracker()

const origSend = pb.send.bind(pb)
pb.send = (async <T>(path: string, options: Parameters<typeof origSend>[1]) => {
    try {
        const result = (await origSend(path, options)) as T
        reachability.record(path, true, null, Date.now())
        // Any successful request proves the server is reachable — recover the
        // signal regardless of tracker streak state (the health-probe poll or
        // another path may have flipped it).
        if (!useConnectivityStore.getState().isServerReachable) {
            useConnectivityStore.getState().setServerReachable(true)
        }
        return result
    } catch (err) {
        if (reachability.record(path, false, err, Date.now()) === 'down') {
            useConnectivityStore.getState().setServerReachable(false)
        }
        throw err
    }
}) as typeof pb.send

export function usePocketBase() {
    return pb
}

// Tear down the live PB session, then drop the resolved address so the
// next gate pass routes to the picker. Order matters: PB's RealtimeService
// auto-reconnects on EventSource error, and reconnect reads PB_SERVER_ADDR
// — clearing the address before disconnecting realtime trips the "address
// not resolved" guard. The auth-store's logout already does the realtime
// teardown + auth clear + resetSessionState (stores + query cache); we
// just need to add the address clear.
export async function disconnectServer() {
    const { useAuthStore } = await import('./stores/auth-store')
    useAuthStore.getState().logout()
    pb.cancelAllRequests()
    const { clearCached, setResolvedAddress } = await import('./server-address')
    await clearCached()
    setResolvedAddress(null)
}

// Route pbtsdb's internal logs somewhere useful. Errors go to Sentry so a
// failed subscription/sync is observable in production instead of being
// silently dropped — the previous empty if/else bodies discarded EVERYTHING,
// errors included, which is the bug this fixes. The debug/info/warn levels are
// subscription-lifecycle chatter with no production value, so they're
// intentionally dropped rather than logged (no unguarded console shipped).
setLogger({
    debug: () => {},
    info: () => {},
    warn: () => {},
    error: (msg, context) => {
        // Stable grouping key; the varying message rides in the Error and the
        // pbtsdb context object rides in `extra`.
        captureException('pbtsdb.error', new Error(msg), context as Record<string, unknown>)
    },
})

// A global query-error handler so failed reads are observable in Sentry — useQuery
// surfaces errors via state rather than throwing, so without this a read failure is
// silent (the component shows its own error UI, but nothing is logged). The queryKey
// is the grouping context. Mutations report via their own onError/handleMutationErrorsWithForm.
const queryClient = new QueryClient({
    queryCache: new QueryCache({
        onError: (error, query) => {
            captureException('query.error', error, { queryKey: query.queryKey })
        },
    }),
})

const newCollection = createCollection<MergedSchema>(pb, queryClient)

const indexing = {
    collectionOptions: {
        autoIndex: 'eager' as const,
        defaultIndexType: BasicIndex,
    },
}

const users = newCollection('users', {
    omitOnInsert: ['created', 'updated', 'password', 'tokenKey'],
    ...indexing,
})

const orgs = newCollection('orgs', {
    omitOnInsert: ['created', 'updated'],
    ...indexing,
})

const user_org = newCollection('user_org', {
    omitOnInsert: ['created', 'updated'],
    expand: {
        user: users,
        org: orgs,
    },
    ...indexing,
})

const settings = newCollection('settings', {
    omitOnInsert: ['created', 'updated'],
    expand: { org: orgs },
    ...indexing,
})

const user_preferences = newCollection('user_preferences', {
    omitOnInsert: ['created', 'updated'],
    expand: { user: users },
    ...indexing,
})

const labels = newCollection('labels', {
    omitOnInsert: ['created', 'updated'],
    expand: { org: orgs, user_org },
    ...indexing,
})

const label_assignments = newCollection('label_assignments', {
    omitOnInsert: ['created', 'updated'],
    expand: { label: labels, user_org },
    ...indexing,
})

const org_pkg_access = newCollection('org_pkg_access', {
    omitOnInsert: ['created', 'updated'],
    expand: { user_org },
    ...indexing,
})

const pkg_registry = newCollection('pkg_registry', {
    omitOnInsert: ['created', 'updated'],
    ...indexing,
})

// Build-history catalog for the Admin panel's Builds tab. Read-only from the
// client (the build pipeline writes it server-side); registered so the tab can
// read it via the pbtsdb store instead of the raw PocketBase client.
const pkg_build = newCollection('pkg_build', {
    omitOnInsert: ['created', 'updated'],
    ...indexing,
})

const org_pkg_enabled = newCollection('org_pkg_enabled', {
    omitOnInsert: ['created', 'updated'],
    expand: { org: orgs },
    ...indexing,
})

// Intent signal core emits on admin org-create so feature packages (mail) can
// provision their own per-org data without core depending on them. See the
// create_org_provisioning migration.
const org_provisioning = newCollection('org_provisioning', {
    omitOnInsert: ['created', 'updated'],
    expand: { org: orgs },
    ...indexing,
})

const super_admins = newCollection('super_admins', {
    omitOnInsert: ['created', 'updated'],
    expand: { user: users },
    ...indexing,
})

// System-wide configuration (Sentry, web-push, mail provider creds). Read/written
// by the /admin Settings console; the server reads it through SystemConfig. See
// the create_system_settings migration.
const system_settings = newCollection('system_settings', {
    omitOnInsert: ['created', 'updated'],
    ...indexing,
})

const audit_logs = newCollection('audit_logs', {
    omitOnInsert: ['created', 'updated'],
    expand: { actor: users },
    ...indexing,
})

const pkg_install_log = newCollection('pkg_install_log', {
    omitOnInsert: ['created', 'updated'],
    expand: { initiated_by: users },
    ...indexing,
})

const notifications = newCollection('notifications', {
    omitOnInsert: ['created', 'updated'],
    expand: { user: users, org: orgs },
    ...indexing,
})
export const notificationsCollection = notifications

// NOTE: the `comment_mentions` store registration was removed in the new
// standalone-core layout because the collection is created by a feature
// package's migration (drive's create_comment_mentions), not core's own — so
// core can't unconditionally register it when that package isn't linked. The
// comment-mutations factory takes the collection as a parameter, so nothing in
// core hard-depends on a core-owned store here. Re-introducing comments as a
// first-class core feature (with its own migration) is tracked as a follow-up.

const coreStores = {
    users,
    orgs,
    user_org,
    settings,
    user_preferences,
    labels,
    label_assignments,
    org_pkg_access,
    pkg_registry,
    pkg_build,
    org_pkg_enabled,
    org_provisioning,
    audit_logs,
    pkg_install_log,
    notifications,
    super_admins,
    system_settings,
}
export type CoreStores = typeof coreStores

const stores = {
    ...coreStores,
    ...buildPackageStores(tinycldConfig, newCollection, coreStores),
}

const { Provider: PBTSDBProvider, useStore } = createReactProvider(stores)

export function getUserFromAuthStore(primaryOrgSlug?: string | null): UserSession | null {
    const authRecord = pb.authStore.record as Users | null
    const authToken = pb.authStore.token

    if (!authRecord || !authToken || !pb.authStore.isValid) {
        return null
    }

    const metadata = (authRecord as Users & { metadata?: Record<string, unknown> }).metadata
    return {
        id: authRecord.id,
        name: authRecord.name,
        email: authRecord.email,
        primaryOrgSlug: primaryOrgSlug ?? undefined,
        isDemo: !!(authRecord as Users & { is_demo?: boolean }).is_demo,
        isBetaTester: !!metadata?.isBetaTester,
    }
}

export async function seedUserOrg(userRecord: Users, orgRecord: Orgs, userOrgRecord: UserOrg) {
    await Promise.all([stores.users.preload(), stores.orgs.preload(), stores.user_org.preload()])
    stores.users.utils?.writeUpsert(userRecord)
    stores.orgs.utils?.writeUpsert(orgRecord)
    stores.user_org.utils?.writeUpsert(userOrgRecord)
}

export async function preloadStores() {
    await Promise.all([
        stores.orgs.preload(),
        stores.user_org.preload(),
        stores.org_pkg_access.preload(),
        stores.pkg_registry.preload(),
        stores.org_pkg_enabled.preload(),
        stores.super_admins.preload(),
    ])
}

export async function fetchAndSeedUserOrg() {
    await Promise.all([stores.users.preload(), stores.orgs.preload(), stores.user_org.preload()])
    const userOrgs = await pb.collection('user_org').getFullList<UserOrg>()
    for (const userOrgRecord of userOrgs) {
        stores.user_org.utils?.writeUpsert(userOrgRecord)
    }
}

export async function clearStores() {
    for (const s of Object.values(stores)) {
        // cleanup() on an already cleaned-up collection throws an invalid
        // status-transition error, and repeated teardowns are a legal path
        // (sign out, then "change server" runs logout() a second time).
        if (s.status === 'cleaned-up') continue
        await s.cleanup()
    }
}

// Single teardown point for everything cached per-session in memory. On web
// the SPA survives logout → next login in the same tab (no reload), so
// anything left behind leaks across users on the same device: the pbtsdb
// stores still hold the previous user's rows, and the React Query cache still
// holds their file token (['pb-files-token'] in use-authed-file-url.ts) —
// which authorizes file reads as that user. Runs from the auth-store's
// logout(), which disconnectServer() also routes through.
//
// Push subscription teardown is NOT done here: it needs the userId and an
// authorized PB session, both of which are gone by the time resetSessionState
// runs (logout() clears pb.authStore first). The auth-store's logout() tears
// down push explicitly, before the auth clear — see teardownPushOnLogout.
export async function resetSessionState() {
    await clearStores()
    queryClient.clear()
}

export { PBTSDBProvider, queryClient, stores, useStore }
