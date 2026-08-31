import AsyncStorage from '@react-native-async-storage/async-storage'
import { Platform } from 'react-native'
import { ApexServerError, looksLikeApexResponse } from './apex'
import { getCoreConfigOptional, registerConfigListener } from './core-config'

const STORAGE_KEY_PREFIX = 'tinycld:server:'

// The public hosted server the demo flow pins. A universal-link demo open
// (tinycld.org/demo → /p/demo) signals wanting the hosted demo, not any
// self-hosted server the user configured, so both the server-address gate (to
// resolve synchronously on a fresh install) and app/p/demo.tsx use this.
export const DEMO_SERVER = 'https://tinycld.org'

function envToAddress(env: string): string | null {
    const config = getCoreConfigOptional()
    if (!config) return null
    if (env === 'web') {
        const fromConfig = config.webShortcut?.()
        if (fromConfig) return fromConfig
        // `window.location` only exists on web. On native, RN defines a partial
        // `window` WITHOUT `location`, so reading `.origin` throws. This branch is
        // reachable on native because the server-exported OTA bundle bakes
        // EXPO_PUBLIC_ENV='web' (it's a web-context export), which drives a native
        // device down here at configureCore() module-init. Gate on Platform, not
        // `typeof window`, and return null so native falls back to the cached
        // address / connect screen rather than crashing.
        if (Platform.OS !== 'web' || typeof window === 'undefined') return null
        return window.location.origin
    }
    return config.serverShortcuts[env] ?? null
}

export function resolveEnvAddress(): string | null {
    // Web is always same-origin: the page is served from the dev proxy (or
    // production app server) which routes /api and /_ to PB. There's no
    // valid reason to point web fetches at a different origin in any env,
    // so we ignore EXPO_PUBLIC_ENV on web and resolve via window.location.
    if (Platform.OS === 'web') return envToAddress('web')

    const env = process.env.EXPO_PUBLIC_ENV
    if (!env) return null
    return envToAddress(env)
}

function storageKey(): string {
    if (Platform.OS === 'web' && typeof window !== 'undefined') {
        return `${STORAGE_KEY_PREFIX}${window.location.origin}`
    }
    return `${STORAGE_KEY_PREFIX}app`
}

export async function readCached(): Promise<string | null> {
    return AsyncStorage.getItem(storageKey())
}

export async function writeCached(address: string): Promise<void> {
    await AsyncStorage.setItem(storageKey(), address)
}

export async function clearCached(): Promise<void> {
    await AsyncStorage.removeItem(storageKey())
}

export function normalizeAddress(input: string): string {
    let addr = input.trim()
    if (!/^https?:\/\//i.test(addr)) addr = `https://${addr}`
    addr = addr.replace(/\/+$/, '')
    return addr
}

// probe is a LIVENESS check: "is the server I already connected to answering?"
// It is polled on a timer by OfflineOverlay and the native connectivity
// detector, so it stays the cheapest possible request and deliberately does not
// inspect the body. Use probeServer() to ADMIT a new address — liveness is the
// wrong question there, because a hosting apex is perfectly alive.
export async function probe(address: string, timeoutMs = 5000): Promise<void> {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), timeoutMs)
    try {
        const res = await fetch(`${address}/api/health`, { signal: controller.signal })
        if (!res.ok) {
            throw new Error(`Server returned HTTP ${res.status}`)
        }
    } finally {
        clearTimeout(timer)
    }
}

// probeServer ADMITS an address: it answers "is there a TinyCld org here I can
// sign into?", which reachability alone cannot.
//
// It asks /api/org-info (unauthenticated, registered unconditionally in
// coreserver.Setup, returns {name}) and requires a JSON object back. A
// hosting apex answers 200 with the org-finder HTML for every path including
// this one, so a status-only check admits it and the app then renders a sign-in
// panel against a host with no PocketBase. Throws ApexServerError for that case
// so the caller can offer the org picker instead of a network error.
export async function probeServer(address: string, timeoutMs = 5000): Promise<void> {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), timeoutMs)
    let res: Response
    let body: string
    try {
        res = await fetch(`${address}/api/org-info`, { signal: controller.signal })
        // Read the body before judging: the apex's HTML arrives with a 200, so
        // status alone cannot tell the two apart.
        body = await res.text()
    } finally {
        clearTimeout(timer)
    }

    if (looksLikeApexResponse(res.headers?.get?.('content-type') ?? '', body)) {
        throw new ApexServerError(address)
    }
    if (!res.ok) {
        throw new Error(`Server returned HTTP ${res.status}`)
    }

    let parsed: unknown
    try {
        parsed = JSON.parse(body)
    } catch {
        throw new Error('That address did not answer like a TinyCld server.')
    }
    // `name` may legitimately be '' (a deployment that never set an app name),
    // so check for the FIELD, not a truthy value.
    if (typeof parsed !== 'object' || parsed === null || !('name' in parsed)) {
        throw new Error('That address did not answer like a TinyCld server.')
    }
}

let resolvedAddress: string | null = null
const listeners = new Set<() => void>()

export function setResolvedAddress(address: string | null): void {
    resolvedAddress = address
    if (address) bindBundleStoreToServer(address)
    for (const listener of listeners) listener()
}

// Point the native bundle store at this server, so anything reading "the current
// bundle" (the boot beacon, Sentry's release/dist) sees THIS server's state, and
// so the key is on disk for the next launch — the native loader runs before the
// JS bridge and can only read what was persisted.
//
// DEFERRED to a microtask, and that is load-bearing, not stylistic. This module
// calls applyEnvAddress() at module-init, so a synchronous body here would run
// during the static import graph — ahead of app/_layout.tsx's
// `import '~/lib/polyfill-crypto'`. Requiring anything from that position drags
// its transitive graph in early too, and @tanstack/db's collection constructor
// calls crypto.randomUUID() at ITS module init, which Hermes has no global for:
// the app dies at launch with "ReferenceError: Property 'crypto' doesn't exist"
// before any handler can report it. Deferring lets the polyfill install first.
//
// Also fail-soft: the updater is a native module absent on web and in tests, and
// nothing here may block the app from connecting.
function bindBundleStoreToServer(address: string): void {
    if (Platform.OS === 'web') return
    queueMicrotask(() => {
        try {
            // Required lazily (see above): a top-level import would pull the
            // native module into every environment that merely resolves an
            // address, and would defeat the deferral this function depends on.
            const AppUpdater = require('app-updater').default as {
                setActiveServer?: (key: string) => void
            }
            const { serverKeyFor } = require('./app-updater/server-key') as {
                serverKeyFor: (a: string) => string
            }
            // Older binaries have no setActiveServer; they keep the single-slot
            // behaviour, which is correct for a single-server install.
            AppUpdater?.setActiveServer?.(serverKeyFor(address))
        } catch {
            // No native module (web stub, tests) — nothing to bind.
        }
    })
}

export function getResolvedAddress(): string | null {
    return resolvedAddress
}

export function subscribeResolvedAddress(listener: () => void): () => void {
    listeners.add(listener)
    return () => {
        listeners.delete(listener)
    }
}

function applyEnvAddress(): void {
    const envAddr = resolveEnvAddress()
    if (envAddr) setResolvedAddress(envAddr)
}

applyEnvAddress()

// Re-resolve once the app calls configureCore — the first pass above runs
// at module load, when config may not be registered yet.
registerConfigListener(applyEnvAddress)
