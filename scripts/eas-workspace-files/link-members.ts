#!/usr/bin/env tsx
// Link every present workspace member into node_modules/@tinycld/<name> so it
// resolves by its package name everywhere (Metro bundling, tsc, runtime).
//
// Why this exists: npm's workspace install symlinks ALL members into the root
// node_modules/@tinycld/ regardless of whether anything depends on them. pnpm
// does not — it only links members that are an actual dependency in the graph.
// Feature siblings (mail, calc, calendar, …) are depended on by nothing (the
// app reaches them only through generated route re-exports that import by
// package name), so pnpm leaves them unlinked and `@tinycld/calc/screens/...`
// fails to resolve.
//
// Rather than hard-code the feature list into app/package.json (which would
// break the lean-shell / partial-checkout guarantee), we mirror npm's behavior
// dynamically: scan for present members and symlink each one. This keeps the
// installed-member set = the set of present sibling dirs, exactly as
// getPackages() discovers it.
//
// Members are linked into BOTH the workspace-root node_modules/@tinycld/ and
// app/node_modules/@tinycld/. Metro resolves deps from the app shell, so the
// app-scoped links are the ones that matter for bundling; the root links cover
// resolution from the workspace root and other members.
import * as fs from 'node:fs'
import * as path from 'node:path'
import { getPackages } from '../tinycld.packages'

const WS_ROOT = path.resolve(import.meta.dirname, '..')

// Map a member's package name to its on-disk sibling dir by scanning the
// workspace root (the dir name need not equal the package name — e.g.
// @tinycld/google-takeout-import lives in google-takeout-import/).
function memberDirsByName(): Map<string, string> {
    const index = new Map<string, string>()
    for (const entry of fs.readdirSync(WS_ROOT)) {
        const dir = path.join(WS_ROOT, entry)
        const pkgJsonPath = path.join(dir, 'package.json')
        try {
            if (!fs.statSync(dir).isDirectory()) continue
            const name = JSON.parse(fs.readFileSync(pkgJsonPath, 'utf8')).name
            if (typeof name === 'string' && name.length > 0 && !index.has(name)) {
                index.set(name, dir)
            }
        } catch {
            // not a dir, or no/unreadable package.json — skip
        }
    }
    return index
}

// Create (or refresh) node_modules/@tinycld/<short> -> <memberDir> as a relative
// symlink inside the given node_modules dir.
function linkInto(nodeModulesDir: string, scopeName: string, targetDir: string): void {
    const scopeDir = path.join(nodeModulesDir, '@tinycld')
    fs.mkdirSync(scopeDir, { recursive: true })
    const linkPath = path.join(scopeDir, scopeName)
    const relTarget = path.relative(scopeDir, targetDir)

    // If a correct symlink already exists, leave it. Otherwise replace whatever
    // is there (stale link, or pnpm's own link for a depended-on member).
    try {
        if (fs.lstatSync(linkPath).isSymbolicLink() && fs.readlinkSync(linkPath) === relTarget) {
            return
        }
        fs.rmSync(linkPath, { recursive: true, force: true })
    } catch {
        // nothing at linkPath yet
    }
    fs.symlinkSync(relTarget, linkPath)
}

function main(): void {
    const names = getPackages() // ['@tinycld/core', '@tinycld/calc', ...]
    const dirs = memberDirsByName()
    const targets = [
        path.join(WS_ROOT, 'node_modules'),
        path.join(WS_ROOT, 'app', 'node_modules'),
    ]

    let linked = 0
    for (const name of names) {
        const short = name.startsWith('@tinycld/') ? name.slice('@tinycld/'.length) : name
        const dir = dirs.get(name)
        if (!dir) {
            console.warn(`[link-members] no sibling dir found for ${name} — skipping`)
            continue
        }
        for (const nm of targets) {
            // Only link where a node_modules already exists (app may be absent
            // in some partial checkouts). The root one always exists post-install.
            if (fs.existsSync(nm)) linkInto(nm, short, dir)
        }
        linked++
    }
    console.log(`[link-members] linked ${linked} workspace member(s) into node_modules/@tinycld/`)
}

main()
