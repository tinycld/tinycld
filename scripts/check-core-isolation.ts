import { execFileSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { getPackages } from '../../tinycld.packages'
import { loadManifest } from './load-manifest'
import { APP_DIR, memberDir } from './paths'

// Core must not know any package.
//
// The app shell, @tinycld/core and the CLI shell have to build, typecheck and
// run with zero feature packages linked, and a package has to be installable
// without a core change. Both break the moment core names a package: a scope
// table that needs a new row per package route, a settings screen that lists
// package collections, a helper that reaches for `/api/<slug>/...`. Each such
// line is a core PR for something the package should own, and each is
// silently wrong for a deployment without that package.
//
// The rule is enforced two ways. The Go scope registry has a guard test
// (TestCoreDeclaresNoPackageScopes) that runs with no features present. This
// script is the broader net: it derives the installed packages' slugs and
// collection names from the workspace and refuses any load-bearing reference
// to them in the shell's runtime code. It is only as strong as the local
// assembly — a workspace with no features has nothing to check — so run it
// with your features linked before opening a core PR.
//
// What counts as a reference (comments are stripped first):
//   - a package route:      "/api/<slug>/"
//   - a package scope:      "<slug>:<capability>"
//   - a package import:     "@tinycld/<slug>"
//   - a package collection: "<collection>" for every collection the schema
//     owns by the <slug>_* naming convention (see docs/packages.md,
//     "Collection naming and package access enforcement")
//
// Only files tracked by git are scanned, so generated output — which is
// SUPPOSED to name every installed package — is excluded by its gitignore.
// Test files are not scanned either: a generator test that uses a real
// package as fixture data is not coupling.
//
// The allowlist below is a debt register, not an escape hatch. Every entry
// must still match something (the script fails when one goes stale, so the
// list only shrinks), and every entry names the registry that should replace
// it. Do not add to it to make a new change pass — give the package a way to
// register whatever core needs to know instead. The oauth registry
// (core/server/oauth/registry.go) is the pattern.
const ALLOWLIST: { file: string; reason: string }[] = [
    {
        file: 'core/server/driveshare/driveshare.go',
        reason: 'drive-item authorization shared by drive, text and calc; needs a core document-access registry',
    },
    {
        file: 'core/server/sharelink/sharelink.go',
        reason: 'public share links are drive-shaped; same registry as driveshare',
    },
    {
        file: 'core/server/notify/comment_mentions.go',
        reason: 'mention targets resolve through drive_items; the target collection should be registered by the package',
    },
    {
        file: 'core/lib/account.ts',
        reason: 'offboarding labels per package collection; should come from offboard.RegisterReassignable',
    },
    {
        file: 'app/a/(app)/settings/audit-log.tsx',
        reason: 'the audit filter lists package collections; should come from audit.RegisterCollection',
    },
    {
        file: 'core/file-viewer/fetch-rendered-html.ts',
        reason: 'render routes per document package; the file viewer should resolve a renderer through the package registry',
    },
    {
        file: 'scripts/reset-demo.ts',
        reason: 'demo reset lists package collections; should walk the schema by the <slug>_ convention',
    },
    {
        file: 'scripts/cli-smoke.ts',
        reason: 'smoke test names package scopes; should read scopes_supported from discovery',
    },
    {
        file: 'scripts/write-workspace-root.ts',
        reason: 'ALL_FEATURES seeds pnpm-workspace.yaml; discoverPresentMembers already finds every present member',
    },
    {
        file: 'core/lib/anon-identity.ts',
        reason: 'anonymous share sessions post to a drive route; the share package should register its session endpoint',
    },
    {
        file: 'core/lib/comments/mutations.ts',
        reason: 'comment mentions target drive_items; the target collection should be registered by the package',
    },
    {
        file: 'core/lib/contacts/use-contact-suggestions.tsx',
        reason: 'address suggestions read the contacts collection when linked; should be a suggestion-source registry',
    },
    {
        file: 'core/lib/editor/use-share-visitor-role.tsx',
        reason: 'visitor roles resolve through drive_shares; same document-access registry as driveshare',
    },
    {
        file: 'core/lib/proxy-image-urls.ts',
        reason: 'remote images proxy through a mail route; the package should register the proxy',
    },
    {
        file: 'core/lib/stores/takeout-import-store.ts',
        reason: "import services are the takeout importer's targets; the store belongs to that package",
    },
    {
        file: 'core/server/blankfile/blankfile.go',
        reason: 'blank files attach to drive_items; same document-access registry as driveshare',
    },
    {
        file: 'app/a/(app)/settings/storage.tsx',
        reason: 'storage settings call a drive route; quota sources should expose usage through core',
    },
]

const SCAN_ROOTS = ['core', 'cli', 'app', 'scripts', 'server']
const SKIP_DIRS = ['__tests__', 'tests', 'testdata']
const SKIP_FILES = new Set(['scripts/check-core-isolation.ts'])
const EXTENSIONS = new Set(['.go', '.ts', '.tsx'])

interface Hit {
    file: string
    line: number
    text: string
}

function escapeRegExp(s: string): string {
    return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// The generated schema's collection map is the authoritative list of
// collections in this assembly; a package owns every one named <slug>_* or
// exactly <slug> (dashes in a slug match underscores in the name).
function packageCollections(slugs: string[]): string[] {
    const schema = fs.readFileSync(path.join(APP_DIR, 'core/types/pbSchema.ts'), 'utf8')
    const names = [...schema.matchAll(/^ {4}([a-z0-9_]+): \{$/gm)].map(m => m[1])
    const prefixes = slugs.map(s => s.replace(/-/g, '_'))
    return names.filter(n => prefixes.some(p => n === p || n.startsWith(`${p}_`)))
}

function buildPattern(slugs: string[], collections: string[]): RegExp {
    const slug = slugs.map(escapeRegExp).join('|')
    const alternatives = [`/api/(?:${slug})/`, `["'\`](?:${slug}):[a-z]`, `@tinycld/(?:${slug})\\b`]
    if (collections.length > 0) {
        alternatives.push(`["'\`](?:${collections.map(escapeRegExp).join('|')})["'\`.]`)
    }
    return new RegExp(alternatives.join('|'))
}

function isTestFile(rel: string): boolean {
    if (SKIP_DIRS.some(d => rel.split('/').includes(d))) return true
    return /_test\.go$|\.test\.tsx?$|\.spec\.tsx?$|\.type-test\.ts$/.test(rel)
}

function trackedSources(): string[] {
    const listed = execFileSync('git', ['ls-files', '--', ...SCAN_ROOTS], {
        cwd: APP_DIR,
        encoding: 'utf8',
    })
    return listed
        .split('\n')
        .filter(rel => rel !== '' && EXTENSIONS.has(path.extname(rel)))
        .filter(rel => !SKIP_FILES.has(rel) && !isTestFile(rel))
        .filter(rel => fs.existsSync(path.join(APP_DIR, rel)))
}

// Strip comments so an explanatory "e.g. drive's /api/drive/share-link" does
// not count. Line comments are cut at `//`; a block comment is dropped from
// `/*` to `*/`. A `//` inside a string ("http://…") also cuts the line, which
// can only hide a hit, never invent one.
function stripComments(source: string): string[] {
    const out: string[] = []
    let inBlock = false
    for (const raw of source.split('\n')) {
        let line = raw
        if (inBlock) {
            const end = line.indexOf('*/')
            if (end === -1) {
                out.push('')
                continue
            }
            line = line.slice(end + 2)
            inBlock = false
        }
        for (;;) {
            const start = line.indexOf('/*')
            if (start === -1) break
            const end = line.indexOf('*/', start + 2)
            if (end === -1) {
                line = line.slice(0, start)
                inBlock = true
                break
            }
            line = line.slice(0, start) + line.slice(end + 2)
        }
        const lineComment = line.indexOf('//')
        if (lineComment !== -1) line = line.slice(0, lineComment)
        out.push(line)
    }
    return out
}

function scan(pattern: RegExp): Hit[] {
    const hits: Hit[] = []
    for (const rel of trackedSources()) {
        const lines = stripComments(fs.readFileSync(path.join(APP_DIR, rel), 'utf8'))
        lines.forEach((text, i) => {
            if (pattern.test(text)) hits.push({ file: rel, line: i + 1, text: text.trim() })
        })
    }
    return hits
}

async function main() {
    const features = getPackages().filter(name => name !== '@tinycld/core')
    const slugs: string[] = []
    for (const name of features) {
        slugs.push((await loadManifest(memberDir(name))).slug)
    }
    if (slugs.length === 0) {
        console.log('check-core-isolation: no feature packages linked, nothing to check')
        return
    }
    const collections = packageCollections(slugs)
    const hits = scan(buildPattern(slugs, collections))

    const allowed = new Map(ALLOWLIST.map(a => [a.file, a]))
    const violations = hits.filter(h => !allowed.has(h.file))
    const stale = ALLOWLIST.filter(a => !hits.some(h => h.file === a.file))

    console.log(
        `check-core-isolation: ${slugs.length} package(s) [${slugs.join(', ')}], ${collections.length} package collection(s), ${hits.length} reference(s) in ${new Set(hits.map(h => h.file)).size} file(s), ${ALLOWLIST.length - stale.length} allowlisted`
    )
    for (const v of violations) {
        console.error(`${v.file}:${v.line}: ${v.text}`)
    }
    if (violations.length > 0) {
        console.error(
            `\ncheck-core-isolation: ${violations.length} reference(s) to a package in core/app/cli runtime code.\n` +
                'Core must not name a package. Give the package a registry to declare what core needs\n' +
                '(core/server/oauth/registry.go is the pattern) rather than adding to the allowlist.'
        )
    }
    for (const s of stale) {
        console.error(
            `check-core-isolation: allowlist entry '${s.file}' no longer matches anything — remove it`
        )
    }
    if (violations.length > 0 || stale.length > 0) process.exit(1)
}

main().catch(err => {
    console.error(err)
    process.exit(1)
})
