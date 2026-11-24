#!/usr/bin/env tsx
/**
 * scaffold-terms-stub.ts — provisions the @tinycld/termsstub package into the
 * workspace that contains the app shell.
 *
 * Why a stub: a package can declare terms and conditions (manifest `terms`),
 * and nobody can use it until the owner accepts them. Core must not name a
 * package, and app-shell CI installs no feature package, so without a package
 * that declares terms the package-terms e2e (tests/e2e/package-terms-*.spec.ts)
 * has nothing to gate. The stub declares terms and one of each surface the gate
 * hides: a nav entry with a route, a settings panel, a help topic and a search
 * adapter.
 *
 * Its seed records a `declined` decision. So on a fresh e2e database the stub
 * is closed AND decided: the owner is not sent to Settings → Package terms on
 * every launch of every other spec (only an undecided package does that).
 *
 * Like scaffold-forms-host-stub.ts this script writes every file itself: no
 * bootstrap preset ships terms, and writing the files directly keeps the
 * fixture free of network access and of template drift.
 *
 * Invocation:
 *   - CI: `pnpm exec tsx tinycld/tests/scripts/scaffold-terms-stub.ts` from the
 *     workspace root, after `pnpm install` and before the bundle build, so the
 *     stub's route is in the exported bundle and its terms in the Go server.
 *   - Local: run once before `tinycld-pkg test:e2e`.
 *
 * Idempotent: re-running re-emits the sources, and installs only when the stub
 * is not yet a workspace member.
 *
 * Fixture identifiers the specs assert on (keep in sync with
 * tests/e2e/package-terms-helpers.ts): slug `termsstub`, name "Terms Stub",
 * terms version `1`, the sentence TERMS_SENTENCE in the terms text, landing
 * testID `termsstub-landing`, settings panel `general` (testID
 * `termsstub-settings-panel`), help topic "Terms stub topic".
 */

import { spawnSync } from 'node:child_process'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const STUB_DIR = 'termsstub'
export const STUB_SLUG = 'termsstub'
export const STUB_PACKAGE = '@tinycld/termsstub'
const TERMS_VERSION = '1'
const TERMS_SENTENCE = 'You agree to feed the stub once a day.'

// Every subprocess finishes in well under a minute; a bound turns a rare
// wedge into a loud failure instead of a silent runner burn.
const SUBPROCESS_TIMEOUT_MS = 5 * 60_000

function workspaceRoot(): string {
    // tinycld/tests/scripts/<this> → tinycld/ → workspace root
    return resolve(fileURLToPath(import.meta.url), '..', '..', '..', '..')
}

function write(path: string, contents: string): void {
    mkdirSync(resolve(path, '..'), { recursive: true })
    writeFileSync(path, contents)
}

function writePackage(stubDir: string): void {
    const src = `./tinycld/${STUB_SLUG}`
    const pkg = {
        name: STUB_PACKAGE,
        version: '0.1.0',
        private: true,
        type: 'module',
        exports: {
            './package.json': './package.json',
            './manifest': './manifest.ts',
            './seed': `${src}/seed.ts`,
            './search-adapter': `${src}/search-adapter.tsx`,
            './screens/*': `${src}/screens/*.tsx`,
            './settings/*': `${src}/settings/*.tsx`,
        },
        peerDependencies: {},
    }
    write(join(stubDir, 'package.json'), `${JSON.stringify(pkg, null, 4)}\n`)
    // Written as biome formats it: the workspace lint covers every member, the stub included.
    write(
        join(stubDir, 'tsconfig.json'),
        `{
    "extends": "@tinycld/core/tsconfig.package-base.json",
    "compilerOptions": {
        "paths": {
            "~/tinycld/${STUB_SLUG}/*": ["./tinycld/${STUB_SLUG}/*"]
        }
    },
    "include": ["tinycld/**/*.ts", "tinycld/**/*.tsx"],
    "exclude": ["node_modules"]
}
`
    )
}

function writeManifest(stubDir: string): void {
    write(
        join(stubDir, 'manifest.ts'),
        `const manifest = {
    name: 'Terms Stub',
    slug: '${STUB_SLUG}',
    version: '0.1.0',
    description: 'app E2E package-terms stub',
    routes: { directory: 'screens' },
    nav: { label: 'Terms Stub', icon: 'file-check', order: 996 },
    terms: { file: 'TERMS.md', version: '${TERMS_VERSION}' },
    settings: [{ slug: 'general', label: 'Terms Stub Panel', component: 'settings/general' }],
    help: { directory: 'help' },
    search: { adapter: 'search-adapter' },
    seed: { script: 'seed' },
}

export default manifest
`
    )
    write(
        join(stubDir, 'TERMS.md'),
        `# Terms Stub terms and conditions

${TERMS_SENTENCE}

1. The stub is a test fixture.
2. It does nothing else.
`
    )
    write(
        join(stubDir, 'help', 'topic.md'),
        `---
title: Terms stub topic
summary: A help topic that shows only when the terms of the stub are accepted.
order: 1
---

To use the terms stub, the owner accepts its terms first.
`
    )
}

function writeSources(stubDir: string): void {
    const dir = join(stubDir, 'tinycld', STUB_SLUG)
    write(
        join(dir, 'screens', 'index.tsx'),
        `import { Text, View } from 'react-native'

export default function TermsStubIndex() {
    return (
        <View className="flex-1 bg-background p-4">
            <Text className="text-foreground" testID="termsstub-landing">
                Terms stub landing
            </Text>
        </View>
    )
}
`
    )
    write(
        join(dir, 'settings', 'general.tsx'),
        `import { Text, View } from 'react-native'

export default function TermsStubPanel() {
    return (
        <View className="p-4">
            <Text className="text-foreground" testID="termsstub-settings-panel">
                Terms stub panel
            </Text>
        </View>
    )
}
`
    )
    write(
        join(dir, 'search-adapter.tsx'),
        `import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useRouter } from 'expo-router'

// The palette calls this for every in-scope package while it is open, so it
// only takes handles. The stub has no server search source: only its scope
// chip matters to the specs.
export function useSearchActions() {
    const router = useRouter()
    const orgHref = useOrgHref()
    return { onSelect: () => router.push(orgHref('${STUB_SLUG}')) }
}
`
    )
    write(
        join(dir, 'seed.ts'),
        `import type PocketBase from 'pocketbase'

const SLUG = '${STUB_SLUG}'

// Records that the owner DECLINED the stub's terms, when nobody decided yet.
// A package with undecided terms sends the owner to Settings → Package terms
// on every launch, which would move every other spec's landing page. The
// package-terms specs put the state they need in place themselves.
export default async function seed(pb: PocketBase): Promise<void> {
    const row = await pb
        .collection('pkg_registry')
        .getFirstListItem(pb.filter('slug = {:slug}', { slug: SLUG }))
    const version = String(row.terms_version ?? '')
    if (version === '' || row.terms_accepted_version === version) return
    const decided = await pb.collection('pkg_terms_decisions').getList(1, 1, {
        filter: pb.filter('pkg = {:slug} && terms_version = {:version}', { slug: SLUG, version }),
    })
    if (decided.totalItems > 0) return
    await pb
        .collection('pkg_terms_decisions')
        .create({ pkg: SLUG, terms_version: version, decision: 'declined' })
}
`
    )
}

/** Registers the stub in package.json workspaces and pnpm-workspace.yaml; true when either changed. */
function ensureMember(wsRoot: string): boolean {
    const path = join(wsRoot, 'package.json')
    const pkg = JSON.parse(readFileSync(path, 'utf8'))
    const workspaces: string[] = Array.isArray(pkg.workspaces) ? pkg.workspaces : []
    const missing = !workspaces.includes(STUB_DIR)
    if (missing) {
        pkg.workspaces = [...workspaces, STUB_DIR]
        writeFileSync(path, `${JSON.stringify(pkg, null, 4)}\n`)
    }
    const added = ensurePnpmMember(join(wsRoot, 'pnpm-workspace.yaml'))
    return missing || added
}

/** Adds `- <STUB_DIR>` to the packages: block. Returns false when already present. */
function ensurePnpmMember(yamlPath: string): boolean {
    const lines = readFileSync(yamlPath, 'utf8').split('\n')
    const pkgIdx = lines.findIndex(l => /^packages:\s*$/.test(l))
    if (pkgIdx === -1) throw new Error(`no packages: block in ${yamlPath}`)
    let lastEntry = pkgIdx
    for (let i = pkgIdx + 1; i < lines.length; i++) {
        const line = lines[i] ?? ''
        if (line.trim() === `- ${STUB_DIR}`) return false
        if (/^\s+-\s+/.test(line)) lastEntry = i
        else if (line.trim() !== '' && !line.startsWith(' ')) break
    }
    const indent = (lines[lastEntry] ?? '').match(/^\s*/)?.[0] ?? '  '
    lines.splice(lastEntry + 1, 0, `${indent}- ${STUB_DIR}`)
    writeFileSync(yamlPath, lines.join('\n'))
    return true
}

function install(wsRoot: string): void {
    // COREPACK_ENABLE_DOWNLOAD_PROMPT=0: corepack's "download pnpm?" prompt is
    // a silent stdin-wait in non-TTY-ish CI.
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
        throw new Error(`pnpm install failed (status ${r.status}); the terms stub will not link`)
    }
}

/**
 * Regenerate so the stub's route, settings panel, help topic, seed and terms
 * land in the generated config and in the server's package list. Without this
 * the package exists on disk and contributes nothing.
 */
function regenerateConfig(wsRoot: string): void {
    const r = spawnSync('pnpm', ['run', 'packages:generate'], {
        cwd: join(wsRoot, 'tinycld'),
        stdio: 'inherit',
        timeout: SUBPROCESS_TIMEOUT_MS,
    })
    if (r.status !== 0) {
        const cause = r.error ? `: ${r.error.message}` : ''
        throw new Error(`packages:generate failed${cause}; the terms stub will not be loaded`)
    }
}

function main(): void {
    const wsRoot = workspaceRoot()
    console.log(`[scaffold-terms-stub] workspace root: ${wsRoot}`)
    const stubDir = join(wsRoot, STUB_DIR)
    writePackage(stubDir)
    writeManifest(stubDir)
    writeSources(stubDir)
    if (ensureMember(wsRoot)) install(wsRoot)
    regenerateConfig(wsRoot)
    console.log(`[scaffold-terms-stub] done — ${STUB_DIR}/ ready at ${stubDir}`)
}

main()
