#!/usr/bin/env tsx
/**
 * scaffold-shortcut-stub.ts — provisions the @tinycld/shortcut-stub
 * package into the workspace that's hosting app.
 *
 * Why a stub: app's keyboard-shortcut tests (open help, navigate via
 * `t <letter>` chord) need a package installed that contributes a nav
 * entry with a shortcut, plus a routable URL to land on. Pointing them
 * at real packages (mail, contacts) couples app's CI to those packages'
 * presence — when a feature's branch breaks, app's CI goes red over a
 * bug that doesn't live in app. The stub registers a minimal package
 * with `nav.shortcut: 'o'` and a single placeholder screen, giving the
 * shortcut tests exactly what they need to verify app's OWN contract:
 * "shortcut chord navigates to the package whose manifest registered it."
 *
 * The script is idempotent. If shortcut-stub already lives in the
 * workspace (e.g. a developer ran tests once locally), we skip the
 * bootstrap call and just re-emit the stub source files so iterating on
 * them doesn't require a clean checkout.
 *
 * Invocation:
 *   - CI: workflow runs `tsx app/tests/scripts/scaffold-shortcut-stub.ts`
 *     from the workspace root after `npm install`.
 *   - Local: developers run it once before `tinycld-pkg test:e2e`.
 *
 * Layout invariant: the workspace root is the parent of app/, and the
 * script always operates on that root, NOT on cwd. Running from inside
 * app/ or the root behaves the same.
 *
 * Pattern source: drive/tests/scripts/scaffold-share-stub.ts. Both
 * scripts use bootstrap's --new --preset settings-only flow then patch
 * the scaffolded manifest + package.json to expose exactly the surface
 * each repo's tests need.
 */

import { execFileSync, spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

// Slug, nav shortcut, and screen path are stable identifiers the
// keyboard-shortcuts spec asserts on. Changing any of these requires
// updating both this script AND tests/e2e/keyboard-shortcuts.spec.ts.
export const STUB_SLUG = 'shortcut-stub'
export const STUB_NAV_SHORTCUT = 'k'
export const STUB_NAV_LABEL = 'Shortcut Stub'
const BOOTSTRAP_VERSION = '@tinycld/bootstrap@2.0.1'

function workspaceRoot(): string {
    // app/tests/scripts/scaffold-shortcut-stub.ts → app/ → workspace root
    return resolve(fileURLToPath(import.meta.url), '..', '..', '..', '..')
}

function ensureBootstrapped(wsRoot: string): void {
    const stubDir = join(wsRoot, STUB_SLUG)
    if (existsSync(stubDir)) {
        console.log(`[scaffold-shortcut-stub] ${STUB_SLUG}/ exists — skipping bootstrap`)
        return
    }
    console.log(`[scaffold-shortcut-stub] running bootstrap to scaffold ${STUB_SLUG}/`)
    // --no-link: bootstrap would otherwise try to assemble app+core (already
    // present) and run npm install. The caller is responsible for the
    // install step, so we skip both.
    execFileSync(
        'npx',
        [
            '--yes',
            BOOTSTRAP_VERSION,
            '--new',
            STUB_SLUG,
            '--yes',
            '--preset',
            'settings-only',
            '--name',
            STUB_NAV_LABEL,
            '--description',
            'app E2E keyboard-shortcut stub',
            '--no-link',
            '--target',
            stubDir,
        ],
        { stdio: 'inherit', cwd: wsRoot }
    )
}

function patchManifest(stubDir: string): void {
    const path = join(stubDir, 'manifest.ts')
    // Wholesale replace. The settings-only preset ships a `settings:
    // [...]` array we don't need. We want only routes + nav with a
    // shortcut, the minimum surface the keyboard-shortcuts spec
    // exercises. Keeping the file shape minimal makes the test fixture
    // legible — anyone reading app's e2e suite can see exactly what
    // the stub contributes.
    const contents = `const manifest = {
    name: '${STUB_NAV_LABEL}',
    slug: '${STUB_SLUG}',
    version: '0.1.0',
    description: 'app E2E keyboard-shortcut stub',
    routes: { directory: 'screens' },
    nav: {
        label: '${STUB_NAV_LABEL}',
        // Any kebab-case lucide icon name from https://lucide.dev/icons
        // works — the generator bundles it on \`npm install\`. We pick
        // 'cloud-rain' deliberately: no first-party package uses it, so
        // the rail rendering this glyph in e2e is positive evidence that
        // manifest-driven icon bundling is working end-to-end.
        icon: 'cloud-rain',
        // High order so the stub sorts below real features in the rail
        // — keeps the visible rail layout stable when both run together.
        order: 999,
        // 'k' is the chord letter the keyboard-shortcuts spec types
        // (\`t k\` jumps here). Picked because no real first-party
        // package claims 'k' as a shortcut, so locally running with
        // contacts/mail/etc. installed alongside the stub doesn't
        // cause a chord-binding collision. MUST stay in sync with
        // STUB_NAV_SHORTCUT in this file AND the assertion in
        // tests/e2e/keyboard-shortcuts.spec.ts.
        shortcut: '${STUB_NAV_SHORTCUT}',
    },
}

export default manifest
`
    writeFileSync(path, contents)
}

function patchPackageJson(stubDir: string): void {
    const path = join(stubDir, 'package.json')
    const pkg = JSON.parse(readFileSync(path, 'utf8'))
    // The settings-only template's exports map doesn't include
    // screens/* — we need it because we add a routes directory above.
    pkg.exports = {
        ...(pkg.exports ?? {}),
        './screens/*': `./tinycld/${STUB_SLUG}/screens/*.tsx`,
    }
    writeFileSync(path, `${JSON.stringify(pkg, null, 4)}\n`)
}

function writeScreens(stubDir: string): void {
    const dir = join(stubDir, 'tinycld', STUB_SLUG, 'screens')
    mkdirSync(dir, { recursive: true })
    // _layout: Expo Router requires a layout file for nested routes.
    // Just a Slot — no chrome of our own; the app shell provides the
    // surrounding navigation.
    writeFileSync(
        join(dir, '_layout.tsx'),
        `import { Slot } from 'expo-router'

export default function StubLayout() {
    return <Slot />
}
`
    )
    // index: the landing page when the shortcut chord lands. Renders
    // one identifiable text node the spec can assert on if needed; in
    // practice the spec just waits for the URL change.
    writeFileSync(
        join(dir, 'index.tsx'),
        `import { Text, View } from 'react-native'

export default function StubIndex() {
    return (
        <View className="flex-1 p-4 bg-background">
            <Text className="text-foreground" data-test-id="shortcut-stub-landing">
                Shortcut stub landing
            </Text>
        </View>
    )
}
`
    )
}

function ensureMember(wsRoot: string): void {
    const path = join(wsRoot, 'package.json')
    const pkg = JSON.parse(readFileSync(path, 'utf8'))
    const workspaces: string[] = Array.isArray(pkg.workspaces) ? pkg.workspaces : []
    if (workspaces.includes(STUB_SLUG)) return
    pkg.workspaces = [...workspaces, STUB_SLUG]
    writeFileSync(path, `${JSON.stringify(pkg, null, 4)}\n`)
    // npm install relinks workspaces. We piggy-back on the caller's
    // install (CI runs npm install before invoking this script) by
    // requiring this script to run AFTER install — but only when adding
    // a brand-new member. The first-time scaffold path needs a second
    // install to wire the symlink; subsequent runs are no-ops.
    const r = spawnSync('npm', ['install'], { cwd: wsRoot, stdio: 'inherit' })
    if (r.status !== 0) {
        throw new Error(
            'npm install (post-member-add) failed; the shortcut-stub symlink under node_modules may be missing'
        )
    }
}

function regenerateConfig(wsRoot: string): void {
    // The generator emits app/tinycld.config.ts from getPackages() output.
    // After adding shortcut-stub to the workspace tree we have to re-run
    // it, otherwise the rail won't render the stub's nav entry and the
    // shortcut binding won't be registered.
    const r = spawnSync('npm', ['run', 'packages:generate'], {
        cwd: join(wsRoot, 'app'),
        stdio: 'inherit',
    })
    if (r.status !== 0) {
        throw new Error('packages:generate failed; shortcut-stub will not be loaded by the app')
    }
}

function main(): void {
    const wsRoot = workspaceRoot()
    console.log(`[scaffold-shortcut-stub] workspace root: ${wsRoot}`)

    ensureBootstrapped(wsRoot)

    const stubDir = join(wsRoot, STUB_SLUG)
    patchManifest(stubDir)
    patchPackageJson(stubDir)
    writeScreens(stubDir)

    ensureMember(wsRoot)
    regenerateConfig(wsRoot)

    console.log(`[scaffold-shortcut-stub] done — ${STUB_SLUG}/ ready at ${stubDir}`)
}

main()
