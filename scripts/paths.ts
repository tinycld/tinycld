import * as fs from 'node:fs'
import * as path from 'node:path'

// All generator paths derive from the app member dir (the dir containing this
// scripts/ folder's parent). TINYCLD_APP_DIR overrides for tests.
export const APP_DIR = process.env.TINYCLD_APP_DIR
    ? path.resolve(process.env.TINYCLD_APP_DIR)
    : path.resolve(import.meta.dirname, '..')

export const WS_ROOT = path.resolve(APP_DIR, '..')
export const GENERATED_DIR = path.join(APP_DIR, 'lib', 'generated')
export const ROUTES_BASE = path.join(APP_DIR, 'app', 'a', '[orgSlug]')
export const PUBLIC_ROUTES_BASE = path.join(APP_DIR, 'app', 'p')
export const SERVER_DIR = path.join(APP_DIR, 'server')
export const MIGRATIONS_DIR = path.join(SERVER_DIR, 'pb_migrations')
export const HOOKS_DIR = path.join(SERVER_DIR, 'pb_hooks')

// Resolve a workspace member's on-disk directory by its package.json name.
//
// Members are flat sibling dirs under the workspace root (core, mail, calc, …),
// but a dir name need not equal the package name (@tinycld/google-takeout-import
// lives in google-takeout-import/). We can't go through node_modules/@tinycld/*:
// npm symlinks every member there, but pnpm only links members that something
// actually depends on — feature siblings (which nothing depends on) are absent.
// So scan the workspace root for the sibling whose package.json name matches,
// which is package-manager-agnostic. Indexed once per process.
let memberDirIndex: Map<string, string> | null = null

function buildMemberDirIndex(): Map<string, string> {
    const index = new Map<string, string>()
    for (const entry of fs.readdirSync(WS_ROOT)) {
        const dir = path.join(WS_ROOT, entry)
        const pkgJsonPath = path.join(dir, 'package.json')
        let name: unknown
        try {
            if (!fs.statSync(dir).isDirectory()) continue
            name = JSON.parse(fs.readFileSync(pkgJsonPath, 'utf8')).name
        } catch {
            continue // not a dir, or no/unreadable package.json
        }
        if (typeof name === 'string' && name.length > 0 && !index.has(name)) {
            index.set(name, dir)
        }
    }
    // @tinycld/core now lives nested inside the tinycld member (at <APP_DIR>/core/),
    // not as a top-level sibling. The top-level scan above won't find it, so look
    // for it explicitly. (Only core is nested; feature siblings stay top-level.)
    if (!index.has('@tinycld/core')) {
        const nestedCore = path.join(APP_DIR, 'core')
        const corePkg = path.join(nestedCore, 'package.json')
        try {
            const name = JSON.parse(fs.readFileSync(corePkg, 'utf8')).name
            if (name === '@tinycld/core') index.set('@tinycld/core', nestedCore)
        } catch {
            // no nested core/package.json (e.g. a synthetic test tree) — leave unset
        }
    }
    return index
}

export function memberDir(packageName: string): string {
    if (!memberDirIndex) memberDirIndex = buildMemberDirIndex()
    const dir = memberDirIndex.get(packageName)
    if (!dir) {
        throw new Error(
            `memberDir: no workspace member named "${packageName}" found under ${WS_ROOT}`
        )
    }
    return dir
}
