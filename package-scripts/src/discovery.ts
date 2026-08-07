import * as fs from 'node:fs'
import * as path from 'node:path'

export interface CurrentPackage {
    dir: string
    name: string
    kind: 'feature' | 'app' | 'core'
}
export interface Discovery {
    workspaceRoot: string
    appDir: string
    currentPackage: CurrentPackage | null
}

// The app-shell member package was renamed "app" → "tinycld". Accept either so
// this shared CLI works against workspaces assembled before or after the rename.
export function isAppShellName(name: string | null | undefined): name is 'tinycld' | 'app' {
    return name === 'tinycld' || name === 'app'
}

function readName(dir: string): string | null {
    try {
        return JSON.parse(fs.readFileSync(path.join(dir, 'package.json'), 'utf8')).name ?? null
    } catch {
        return null
    }
}

function hasManifest(dir: string): boolean {
    return (
        fs.existsSync(path.join(dir, 'manifest.ts')) || fs.existsSync(path.join(dir, 'manifest.js'))
    )
}

// Resolve the real path of the nearest existing ancestor.
function realpathExisting(p: string): string {
    let dir = path.resolve(p)
    while (!fs.existsSync(dir)) {
        const parent = path.dirname(dir)
        if (parent === dir) return dir
        dir = parent
    }
    return fs.realpathSync(dir)
}

// Walk up until a package.json with a `workspaces` field is found.
function findWorkspaceRoot(start: string): string {
    let dir = realpathExisting(start)
    while (true) {
        const pj = path.join(dir, 'package.json')
        if (fs.existsSync(pj)) {
            try {
                if (JSON.parse(fs.readFileSync(pj, 'utf8')).workspaces) return dir
            } catch {
                // keep walking
            }
        }
        const parent = path.dirname(dir)
        if (parent === dir) throw new Error(`No workspace root found above ${start}`)
        dir = parent
    }
}

// The app shell = the workspace member whose package.json name is "tinycld"
// (formerly "app"). A workspace can hold SEVERAL checkouts of the app repo at
// once (git worktrees like tinycld-cli-wt beside the main tinycld/ dir), so a
// bare name scan is ambiguous. Resolution order:
//   1. TINYCLD_APP_DIR — the explicit override every generator script honors.
//   2. An app-shell ancestor of cwd — running from inside a checkout targets
//      THAT checkout.
//   3. The checkout the workspace is actually wired to, identified by where
//      the root node_modules/@tinycld/core symlink points (link-members
//      creates it from the active app dir).
//   4. First name match, for workspaces with a single checkout and no install.
function findAppDir(workspaceRoot: string, cwd: string): string {
    if (process.env.TINYCLD_APP_DIR) return path.resolve(process.env.TINYCLD_APP_DIR)

    let dir = realpathExisting(cwd)
    while (dir !== workspaceRoot && path.dirname(dir) !== dir) {
        if (path.dirname(dir) === workspaceRoot && isAppShellName(readName(dir))) return dir
        dir = path.dirname(dir)
    }

    const candidates: string[] = []
    for (const entry of fs.readdirSync(workspaceRoot)) {
        const candidate = path.join(workspaceRoot, entry)
        try {
            if (fs.statSync(candidate).isDirectory() && isAppShellName(readName(candidate))) {
                candidates.push(candidate)
            }
        } catch {
            // skip
        }
    }
    if (candidates.length > 1) {
        try {
            const linkedCore = fs.realpathSync(
                path.join(workspaceRoot, 'node_modules', '@tinycld', 'core')
            )
            const wired = candidates.find(c =>
                linkedCore.startsWith(`${fs.realpathSync(c)}${path.sep}`)
            )
            if (wired) return wired
        } catch {
            // no symlink yet (pre-install) — fall through
        }
    }
    if (candidates.length > 0) return candidates[0]
    throw new Error(`No app shell (member named "tinycld") under ${workspaceRoot}`)
}

// The current scope target = nearest ancestor of cwd that is a feature package
// (has manifest.ts), the app shell (name "app"), or core (name "@tinycld/core").
// core, like the app shell, has no manifest.ts — it's the shared lib, not a
// feature — so it's matched by name.
function findCurrentPackage(start: string, appDir: string): CurrentPackage | null {
    let dir = realpathExisting(start)
    const root = path.dirname(appDir) // workspace root
    while (true) {
        if (fs.existsSync(path.join(dir, 'package.json'))) {
            const name = readName(dir)
            if (dir === appDir && isAppShellName(name)) return { dir, name, kind: 'app' }
            if (name === '@tinycld/core') return { dir, name, kind: 'core' }
            if (hasManifest(dir) && name) return { dir, name, kind: 'feature' }
        }
        const parent = path.dirname(dir)
        if (parent === dir || dir === root) break
        dir = parent
    }
    return null
}

export function discover(cwd: string = process.cwd()): Discovery {
    const workspaceRoot = findWorkspaceRoot(cwd)
    const appDir = findAppDir(workspaceRoot, cwd)
    const currentPackage = findCurrentPackage(cwd, appDir)
    return { workspaceRoot, appDir: fs.realpathSync(appDir), currentPackage }
}
