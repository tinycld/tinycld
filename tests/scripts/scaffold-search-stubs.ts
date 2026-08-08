#!/usr/bin/env tsx
/**
 * scaffold-search-stubs.ts — provisions two searchable stub packages into the
 * workspace hosting the app shell, so the palette's e2e can exercise
 * cross-package search without any real feature package installed.
 *
 * Why stubs: the palette spec used to drive `navigateToPackage(page, 'cards')`
 * and assert on seeded cards/drive/mail rows. App-shell CI assembles app+core
 * ONLY, so every one of those tests failed on a package that was never there —
 * 13 red tests describing nothing about the app's own behaviour. Worse, when the
 * packages *are* installed locally the suite silently depends on their seed
 * fixtures, so an unrelated change to mail's seed data turns app's CI red.
 *
 * Two stubs, not one: the interesting palette behaviours are all cross-package
 * — chips scoping to a subset, section headings appearing at 2+ chips and
 * collapsing to a flat badged list at zero, and score ordering that has to hold
 * across sources. One stub cannot express any of them.
 *
 * The rows are canned in Go rather than seeded into PocketBase. A search Source
 * is just a function returning rows (core/server/search), so a stub needs no
 * collection, no migration and no seed script to be fully searchable — and
 * canned rows make the fixtures the spec asserts on explicit and stable instead
 * of implied by another repo's seed data.
 *
 * Invocation mirrors scaffold-shortcut-stub.ts:
 *   - CI: `tsx tinycld/tests/scripts/scaffold-search-stubs.ts` from the
 *     workspace root, after `pnpm install`.
 *   - Local: run once before `tinycld-pkg test:e2e`.
 *
 * Idempotent: re-running re-emits sources and skips the bootstrap.
 *
 * Pattern source: tests/scripts/scaffold-shortcut-stub.ts. This script adds one
 * thing that one doesn't — a Go module per stub, linked by the generator the
 * same way any feature package's server is.
 */

import { execFileSync, spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

interface StubSpec {
    slug: string
    label: string
    /** Mirrors nav.order — the cross-package ranking tie-break the spec asserts. */
    order: number
    icon: string
    /** Canned rows this stub's search source returns, filtered by term in Go. */
    rows: { id: string; title: string; subtitle: string; meta: string }[]
}

/**
 * Both stubs deliberately carry a row titled "Onboarding …" so a single query
 * hits both packages — that is what makes grouping, badges and cross-package
 * ordering testable. Alpha's row is a title PREFIX match for "onboarding" while
 * Beta's is a mid-title substring, so score ordering (prefix outranks
 * substring) is observable independently of nav.order.
 *
 * Titles must stay in sync with tests/e2e/search-palette.spec.ts.
 */
const STUBS: StubSpec[] = [
    {
        slug: 'search-alpha',
        label: 'Search Alpha',
        order: 990,
        icon: 'flask-conical',
        rows: [
            {
                id: 'alpha-1',
                title: 'Onboarding checklist',
                subtitle: 'Alpha onboarding subtitle',
                meta: '2026-01-01',
            },
            {
                id: 'alpha-2',
                title: 'Quarterly roadmap review',
                subtitle: 'Alpha roadmap subtitle',
                meta: '2026-01-02',
            },
            {
                // Exercises the mid-token-hyphen rule: parseQuery keeps
                // "roadmap-2026" literal rather than reading "-2026" as an
                // exclusion, and the aggregator's sanitizer AND-s the two terms.
                id: 'alpha-3',
                title: 'Roadmap 2026 planning',
                subtitle: 'Alpha planning subtitle',
                meta: '2026-01-03',
            },
        ],
    },
    {
        slug: 'search-beta',
        label: 'Search Beta',
        order: 991,
        icon: 'beaker',
        rows: [
            {
                // Substring, not prefix — must rank below Alpha's prefix match.
                id: 'beta-1',
                title: 'Design review: new onboarding flow',
                subtitle: 'Beta onboarding subtitle',
                meta: '2026-02-01',
            },
            {
                // Matches "review" WITHOUT "onboarding", so `review -onboarding`
                // is a real discriminator: this row survives the exclusion and
                // beta-1 does not.
                id: 'beta-2',
                title: 'Budget review',
                subtitle: 'Beta budget subtitle',
                meta: '2026-02-02',
            },
        ],
    },
]

// Pin bootstrap so an unrelated release cannot turn this fixture red. Matches
// the pin in scaffold-shortcut-stub.ts; bump both together.
const BOOTSTRAP_VERSION = '@tinycld/bootstrap@2.4.0'
const SUBPROCESS_TIMEOUT_MS = 5 * 60_000

interface GoVersions {
    go: string
    pocketbase: string
}

/**
 * Read the Go and PocketBase versions the ecosystem actually uses out of core's
 * go.mod rather than hardcoding them.
 *
 * A Go workspace resolves ONE version per module across every `use`d member, so
 * a stub pinning its own PocketBase changes what the whole server builds
 * against. Hardcoded versions here would drift silently the next time core
 * upgrades, and the failure would surface as a confusing build error in an
 * unrelated package rather than as "the fixture is stale".
 */
function goVersions(wsRoot: string): GoVersions {
    const modPath = join(wsRoot, 'tinycld', 'core', 'server', 'go.mod')
    const mod = readFileSync(modPath, 'utf8')
    const go = mod.match(/^go\s+(\S+)/m)?.[1]
    const pocketbase = mod.match(/github\.com\/pocketbase\/pocketbase\s+(v\S+)/)?.[1]
    if (!go || !pocketbase) {
        throw new Error(`could not read go/pocketbase versions from ${modPath}`)
    }
    return { go, pocketbase }
}

function workspaceRoot(): string {
    // tinycld/tests/scripts/<this> → tinycld/ → workspace root
    return resolve(fileURLToPath(import.meta.url), '..', '..', '..', '..')
}

function ensureBootstrapped(wsRoot: string, stub: StubSpec): void {
    const stubDir = join(wsRoot, stub.slug)
    if (existsSync(stubDir)) {
        console.log(`[scaffold-search-stubs] ${stub.slug}/ exists — skipping bootstrap`)
        return
    }
    console.log(`[scaffold-search-stubs] scaffolding ${stub.slug}/`)
    execFileSync(
        'npx',
        [
            '--yes',
            BOOTSTRAP_VERSION,
            '--new',
            stub.slug,
            '--yes',
            '--preset',
            'settings-only',
            '--name',
            stub.label,
            '--description',
            'app E2E search stub',
            '--no-link',
            '--target',
            stubDir,
        ],
        { stdio: 'inherit', cwd: wsRoot, timeout: SUBPROCESS_TIMEOUT_MS }
    )
}

function patchManifest(stubDir: string, stub: StubSpec): void {
    // Wholesale replace: the settings-only preset ships a settings array the
    // palette tests don't use. routes + nav + server is the whole surface.
    const contents = `const manifest = {
    name: '${stub.label}',
    slug: '${stub.slug}',
    version: '0.1.0',
    description: 'app E2E search stub',
    routes: { directory: 'screens' },
    nav: {
        label: '${stub.label}',
        icon: '${stub.icon}',
        // High order keeps the stubs below real features in the rail when a
        // developer runs with real packages installed alongside them.
        order: ${stub.order},
    },
    server: { package: 'server', module: 'tinycld.org/packages/${stub.slug}' },
    // The Go source makes the stub searchable; this makes it visible TO the
    // palette. deriveSearchPackages reads the manifest, not the server
    // registry, so without this the stub returns rows the palette has no chip
    // for and no onSelect to run.
    search: { adapter: 'search-adapter' },
}

export default manifest
`
    writeFileSync(join(stubDir, 'manifest.ts'), contents)
}

function patchPackageJson(stubDir: string, stub: StubSpec): void {
    const path = join(stubDir, 'package.json')
    const pkg = JSON.parse(readFileSync(path, 'utf8'))
    pkg.exports = {
        ...(pkg.exports ?? {}),
        './screens/*': `./tinycld/${stub.slug}/screens/*.tsx`,
        // Resolves the manifest's `search.adapter` subpath.
        './search-adapter': `./tinycld/${stub.slug}/search-adapter.tsx`,
    }
    writeFileSync(path, `${JSON.stringify(pkg, null, 4)}\n`)
}

/**
 * The pinned bootstrap still emits `'../app/vitest.config'` from before the
 * shell was renamed to tinycld/. Left alone the stub can't resolve its config
 * and `tinycld-pkg check --all` fails on a stale template. Drop when the pin
 * moves past the rename.
 */
function patchVitestConfig(stubDir: string): void {
    const path = join(stubDir, 'vitest.config.ts')
    if (!existsSync(path)) return
    const contents = readFileSync(path, 'utf8')
    const patched = contents.replace("'../app/vitest.config'", "'../tinycld/vitest.config'")
    if (patched !== contents) writeFileSync(path, patched)
}

function writeScreens(stubDir: string, stub: StubSpec): void {
    const dir = join(stubDir, 'tinycld', stub.slug, 'screens')
    mkdirSync(dir, { recursive: true })
    writeFileSync(
        join(dir, '_layout.tsx'),
        `import { Slot } from 'expo-router'

export default function StubLayout() {
    return <Slot />
}
`
    )
    writeFileSync(
        join(dir, 'index.tsx'),
        `import { Text, View } from 'react-native'

export default function StubIndex() {
    return (
        <View className="flex-1 p-4 bg-background">
            <Text className="text-foreground" testID="${stub.slug}-landing">
                ${stub.label} landing
            </Text>
        </View>
    )
}
`
    )
    // The palette navigates to <slug>/<row id> when a row is selected, so the
    // Enter-navigates test needs a route to land on. A [id] screen echoes the
    // id, which is what the spec asserts — proving the row carried its own id
    // through the aggregator rather than the palette navigating to a fixed URL.
    writeFileSync(
        join(dir, '[id].tsx'),
        `import { useLocalSearchParams } from 'expo-router'
import { Text, View } from 'react-native'

export default function StubDetail() {
    const { id } = useLocalSearchParams<{ id?: string }>()
    return (
        <View className="flex-1 p-4 bg-background">
            <Text className="text-foreground" testID="${stub.slug}-detail">
                {id ?? ''}
            </Text>
        </View>
    )
}
`
    )
}

/**
 * Emit the stub's search adapter: the client half the palette needs.
 *
 * Row shaping lives in Go (see writeGoServer); all that remains client-side is
 * selection, which needs a router the server cannot have. Navigating to
 * `<slug>/<row id>` is what lets the Enter test assert the row carried its own
 * id through the aggregator.
 */
function writeSearchAdapter(stubDir: string, stub: StubSpec): void {
    const dir = join(stubDir, 'tinycld', stub.slug)
    mkdirSync(dir, { recursive: true })
    writeFileSync(
        join(dir, 'search-adapter.tsx'),
        `import { useOrgHref } from '@tinycld/core/lib/org-routes'
import type { SearchRow } from '@tinycld/core/lib/search/types'
import { useRouter } from 'expo-router'

// The palette calls this for every in-scope package while it is open, so it
// only takes handles — no fetching, no subscriptions.
export function useSearchActions() {
    const router = useRouter()
    const orgHref = useOrgHref()

    return {
        onSelect: (row: SearchRow) => {
            router.push(orgHref(\`${stub.slug}/\${row.id}\`))
        },
    }
}
`
    )
}

/**
 * Emit the stub's Go module: a search Source over canned rows.
 *
 * Matching is a case-insensitive substring over title+subtitle, ANDed across
 * include terms and negated across excludes. That is deliberately not FTS — the
 * point is to exercise the AGGREGATOR (fan-out, per-source scoping, merge,
 * scoring, grouping), and a real FTS index would add a migration and seed data
 * whose behaviour the spec would then be asserting instead.
 */
function writeGoServer(stubDir: string, stub: StubSpec, versions: GoVersions): void {
    const dir = join(stubDir, 'server')
    mkdirSync(dir, { recursive: true })

    // Versions must match what the other members pin (see cards/server/go.mod):
    // the generated go.work `use`s every member, and a Go workspace resolves one
    // version per module across all of them, so a stub pinning a different
    // PocketBase silently changes what the whole server builds against — or
    // fails the build outright once the proxy is consulted for a version the
    // vendored fork doesn't provide. `go mod tidy` cannot maintain this (it
    // resolves before go.work's replace, so `tinycld.org/core v0.0.0` hits the
    // proxy), which is why it is written out literally here.
    writeFileSync(
        join(dir, 'go.mod'),
        `module tinycld.org/packages/${stub.slug}

go ${versions.go}

require (
	github.com/pocketbase/pocketbase ${versions.pocketbase}
	tinycld.org/core v0.0.0
)
`
    )

    const rowLiterals = stub.rows
        .map(
            r =>
                `\t{ID: ${JSON.stringify(r.id)}, Title: ${JSON.stringify(r.title)}, ` +
                `Subtitle: ${JSON.stringify(r.subtitle)}, Meta: ${JSON.stringify(r.meta)}},`
        )
        .join('\n')

    writeFileSync(
        join(dir, 'register.go'),
        `// Package stub is an E2E fixture: a search source over canned rows.
//
// It exists so the app shell's palette e2e can exercise cross-package search
// with no real feature package installed. See
// tinycld/tests/scripts/scaffold-search-stubs.ts for why.
package stub

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/search"
)

var rows = []search.Row{
${rowLiterals}
}

// Register is the entry point the generated package_extensions.go calls.
func Register(app core.App) error {
	search.RegisterSources(search.Source{
		Slug:  ${JSON.stringify(stub.slug)},
		Label: ${JSON.stringify(stub.label)},
		Order: ${stub.order},
		// No scopes: a session-authenticated caller can search the stub. An
		// OAuth token cannot, which keeps the fixture out of scope-filtering
		// assertions that belong to real packages.
		Search: searchRows,
	})
	return nil
}

// searchRows matches every include term and no exclude term against the row's
// visible text. userID is ignored: canned rows belong to whoever asks, because
// per-user scoping is each real package's own query to enforce and repeating a
// fake version of it here would assert nothing about the aggregator.
func searchRows(_ core.App, _ string, q search.Query) (search.Result, error) {
	matched := make([]search.Row, 0, len(rows))
	for _, row := range rows {
		if matches(row, q) {
			r := row
			matched = append(matched, r)
		}
	}
	total := len(matched)
	if q.Limit > 0 && len(matched) > q.Limit {
		matched = matched[:q.Limit]
	}
	return search.Result{Rows: matched, Total: total}, nil
}

func matches(row search.Row, q search.Query) bool {
	haystack := strings.ToLower(row.Title + " " + row.Subtitle)
	for _, term := range q.Include {
		if !strings.Contains(haystack, strings.ToLower(term)) {
			return false
		}
	}
	for _, term := range q.Exclude {
		if strings.Contains(haystack, strings.ToLower(term)) {
			return false
		}
	}
	return true
}
`
    )
}

function ensureMember(wsRoot: string, slugs: string[]): boolean {
    const path = join(wsRoot, 'package.json')
    const pkg = JSON.parse(readFileSync(path, 'utf8'))
    const workspaces: string[] = Array.isArray(pkg.workspaces) ? pkg.workspaces : []
    const missing = slugs.filter(s => !workspaces.includes(s))
    if (missing.length > 0) {
        pkg.workspaces = [...workspaces, ...missing]
        writeFileSync(path, `${JSON.stringify(pkg, null, 4)}\n`)
    }
    let addedAny = false
    for (const slug of slugs) {
        if (ensurePnpmMember(join(wsRoot, 'pnpm-workspace.yaml'), slug)) addedAny = true
    }
    return missing.length > 0 || addedAny
}

/** Adds `- <slug>` to the packages: block. Returns false when already present. */
function ensurePnpmMember(yamlPath: string, slug: string): boolean {
    const yaml = readFileSync(yamlPath, 'utf8')
    const lines = yaml.split('\n')
    const pkgIdx = lines.findIndex(l => /^packages:\s*$/.test(l))
    if (pkgIdx === -1) throw new Error(`no packages: block in ${yamlPath}`)
    let lastEntry = pkgIdx
    for (let i = pkgIdx + 1; i < lines.length; i++) {
        const line = lines[i] ?? ''
        if (line.trim() === `- ${slug}`) return false
        if (/^\s+-\s+/.test(line)) lastEntry = i
        else if (line.trim() !== '' && !line.startsWith(' ')) break
    }
    // Match the indentation of the entry we're inserting after rather than
    // hard-coding it: pnpm-workspace.yaml uses two spaces, and a four-space
    // entry (what a hard-coded guess produced) parses but reads as a nested list.
    const indent = (lines[lastEntry] ?? '').match(/^\s*/)?.[0] ?? '  '
    lines.splice(lastEntry + 1, 0, `${indent}- ${slug}`)
    writeFileSync(yamlPath, lines.join('\n'))
    return true
}

function install(wsRoot: string): void {
    // COREPACK_ENABLE_DOWNLOAD_PROMPT=0: corepack's "download pnpm?" prompt is
    // a silent stdin-wait in non-TTY-ish CI, one of the ways this hangs mutely.
    const env = { ...process.env, COREPACK_ENABLE_DOWNLOAD_PROMPT: '0' }
    spawnSync('corepack', ['enable'], {
        cwd: wsRoot,
        stdio: 'inherit',
        timeout: SUBPROCESS_TIMEOUT_MS,
        env,
    })
    const r = spawnSync('pnpm', ['install', '--no-frozen-lockfile'], {
        cwd: wsRoot,
        stdio: 'inherit',
        timeout: SUBPROCESS_TIMEOUT_MS,
        env,
    })
    if (r.status !== 0) {
        throw new Error(`pnpm install failed (status ${r.status}); search stubs will not link`)
    }
}

/**
 * Regenerate so the stubs' routes, nav entries and Go registration land in the
 * generated config the app and server read. Without this the packages exist on
 * disk and contribute nothing.
 */
function regenerateConfig(wsRoot: string): void {
    const r = spawnSync('pnpm', ['run', 'packages:generate'], {
        cwd: join(wsRoot, 'tinycld'),
        stdio: 'inherit',
        timeout: SUBPROCESS_TIMEOUT_MS,
    })
    if (r.status !== 0) {
        const cause = r.error ? `: ${r.error.message}` : ''
        throw new Error(`packages:generate failed${cause}; search stubs will not be loaded`)
    }
}

function main(): void {
    const wsRoot = workspaceRoot()
    console.log(`[scaffold-search-stubs] workspace root: ${wsRoot}`)

    const versions = goVersions(wsRoot)

    for (const stub of STUBS) {
        ensureBootstrapped(wsRoot, stub)
        const stubDir = join(wsRoot, stub.slug)
        patchManifest(stubDir, stub)
        patchPackageJson(stubDir, stub)
        patchVitestConfig(stubDir)
        writeScreens(stubDir, stub)
        writeSearchAdapter(stubDir, stub)
        writeGoServer(stubDir, stub, versions)
    }

    // Install once for both stubs rather than per-stub: the first-time scaffold
    // needs an install to wire symlinks, and doing it twice doubles the slowest
    // step of this script for no benefit.
    const slugs = STUBS.map(s => s.slug)
    if (ensureMember(wsRoot, slugs)) {
        install(wsRoot)
    }
    regenerateConfig(wsRoot)

    console.log(`[scaffold-search-stubs] done — ${slugs.join(', ')}`)
}

main()
