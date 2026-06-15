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
// tinycld/node_modules/@tinycld/. Metro resolves deps from the tinycld member
// (the app shell), so the tinycld-scoped links are the ones that matter for
// bundling; the root links cover resolution from the workspace root and other
// members.
import * as fs from 'node:fs'
import * as path from 'node:path'
import { getPackages } from '../tinycld.packages'

const WS_ROOT = path.resolve(import.meta.dirname, '..')

// Absolute path to the app shell dir, which holds the nested core/,
// package-scripts/, and the generated output. Normally <WS_ROOT>/tinycld, but
// EAS cloud builds clone the shell into a dir named 'build', so the EAS install
// script exports TINYCLD_APP_DIR. Resolved as an absolute path to match the
// generator's scripts/paths.ts contract for the same env var.
const APP_DIR = process.env.TINYCLD_APP_DIR
    ? path.resolve(process.env.TINYCLD_APP_DIR)
    : path.join(WS_ROOT, 'tinycld')

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
    // @tinycld/core lives nested in the app shell (<WS_ROOT>/<APP_DIR>/core/);
    // the top-level scan won't find it. Register it explicitly so the
    // node_modules/@tinycld/core symlink is still created.
    if (!index.has('@tinycld/core')) {
        const nestedCore = path.join(APP_DIR, 'core')
        try {
            const name = JSON.parse(
                fs.readFileSync(path.join(nestedCore, 'package.json'), 'utf8')
            ).name
            if (name === '@tinycld/core') index.set('@tinycld/core', nestedCore)
        } catch {
            // no nested core — skip
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

const GITIGNORE_BEGIN = '# >>> tinycld members (auto-managed by link-members.ts) >>>'
const GITIGNORE_END = '# <<< tinycld members <<<'

// Keep the workspace-root .gitignore's member list in sync with the
// independent repos present on disk. Every top-level dir that is its own git
// repo or carries a package.json (members like mail/drive, plus sibling repos
// like bootstrap/utils/web) must never be tracked by the workspace repo — each
// has its own history + remote, and the workspace repo commits only
// coordination files. Nested members (e.g. tinycld/core) are already covered by
// ignoring /tinycld/. The block is delimited so we rewrite only between the
// markers and never clobber hand-written rules (.env, node_modules, scratch).
function discoverSiblingRepos(): string[] {
    const names = new Set<string>()
    for (const entry of fs.readdirSync(WS_ROOT)) {
        if (entry === 'node_modules') continue
        const dir = path.join(WS_ROOT, entry)
        try {
            if (!fs.statSync(dir).isDirectory()) continue
        } catch {
            continue
        }
        const isRepo = fs.existsSync(path.join(dir, '.git'))
        const hasPkg = fs.existsSync(path.join(dir, 'package.json'))
        if (isRepo || hasPkg) names.add(`/${entry}/`)
    }
    return [...names].sort()
}

function syncGitignore(): void {
    const topLevel = discoverSiblingRepos()
    if (topLevel.length === 0) return

    const block = [GITIGNORE_BEGIN, ...topLevel, GITIGNORE_END].join('\n')
    const gitignorePath = path.join(WS_ROOT, '.gitignore')
    let existing = ''
    try {
        existing = fs.readFileSync(gitignorePath, 'utf8')
    } catch {
        // no .gitignore yet — we'll create one
    }

    const beginIdx = existing.indexOf(GITIGNORE_BEGIN)
    const endIdx = existing.indexOf(GITIGNORE_END)
    let next: string
    if (beginIdx !== -1 && endIdx !== -1 && endIdx > beginIdx) {
        // Replace the existing managed block in place.
        const before = existing.slice(0, beginIdx)
        const after = existing.slice(endIdx + GITIGNORE_END.length)
        next = `${before}${block}${after}`
    } else {
        // Append a fresh block, separated by a blank line if there's prior content.
        const sep = existing.length > 0 && !existing.endsWith('\n\n') ? '\n\n' : ''
        const prefix = existing.length > 0 && !existing.endsWith('\n') ? '\n' : ''
        next = `${existing}${prefix}${sep}${block}\n`
    }

    if (next !== existing) {
        fs.writeFileSync(gitignorePath, next)
        console.log(`[link-members] synced ${topLevel.length} member(s) into .gitignore`)
    }
}

function main(): void {
    const names = getPackages() // ['@tinycld/core', '@tinycld/calc', ...]
    const dirs = memberDirsByName()
    syncGitignore()
    const targets = [path.join(WS_ROOT, 'node_modules'), path.join(APP_DIR, 'node_modules')]

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

    // @tinycld/app-generated is NOT a workspace member (it's the generator's
    // output dir, tinycld/lib/generated/). Link it explicitly so core's
    // `@tinycld/app-generated/*` imports resolve by name from any consumer that
    // pulls core in via its exports map. Skip if the generated dir doesn't exist
    // yet (generator runs in the same postinstall, but ordering/partial runs vary).
    const appGeneratedDir = path.join(APP_DIR, 'lib', 'generated')
    if (fs.existsSync(appGeneratedDir)) {
        for (const nm of targets) {
            if (fs.existsSync(nm)) linkInto(nm, 'app-generated', appGeneratedDir)
        }
    } else {
        console.warn(
            '[link-members] tinycld/lib/generated not present — skipping @tinycld/app-generated link'
        )
    }

    console.log(`[link-members] linked ${linked} workspace member(s) into node_modules/@tinycld/`)
}

main()
