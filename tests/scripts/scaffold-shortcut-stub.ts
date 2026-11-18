#!/usr/bin/env tsx
/**
 * scaffold-shortcut-stub.ts — provisions the @tinycld/shortcut-stub
 * package into the workspace that contains the app.
 *
 * Why a stub: app's keyboard-shortcut tests (open help, navigate via
 * `t <letter>` chord) need a package installed that contributes a nav
 * entry with a shortcut, plus a routable URL to land on. Pointing them
 * at real packages (mail, contacts) couples app's CI to those packages'
 * presence — when a feature's branch breaks, app's CI goes red over a
 * bug that doesn't live in app. The stub registers a minimal package
 * with its own `nav.shortcut` and a single placeholder screen, giving the
 * shortcut tests exactly what they need to verify app's OWN contract:
 * "shortcut chord navigates to the package whose manifest registered it."
 *
 * The script is idempotent. If shortcut-stub already lives in the
 * workspace (e.g. a developer ran tests once locally), we skip the
 * bootstrap call and just re-emit the stub source files so iterating on
 * them doesn't require a clean checkout.
 *
 * Invocation:
 *   - CI: workflow runs `tsx tinycld/tests/scripts/scaffold-shortcut-stub.ts`
 *     from the workspace root after `pnpm install`.
 *   - Local: developers run it once before `tinycld-pkg test:e2e`.
 *
 * Layout invariant: the workspace root is the parent of tinycld/ (the merged
 * app shell + core member), and the script always operates on that root, NOT on
 * cwd. Running from inside tinycld/ or the root behaves the same.
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
import { patchTsconfig, patchVitestConfig } from './stub-template-patches'

// Slug, nav shortcut, and screen path are stable identifiers the
// keyboard-shortcuts spec asserts on. Changing any of these requires
// updating both this script AND tests/e2e/keyboard-shortcuts.spec.ts.
export const STUB_SLUG = 'shortcut-stub'
// Must not collide with any real package's nav.shortcut, or `t <letter>`
// becomes ambiguous and the chord test fails on whichever registration the
// matcher happens to reach first. This previously read 'k', which boards later
// claimed. Real letters in use: c d k m o s t — 'z' is deliberately far from
// anything a feature would want.
export const STUB_NAV_SHORTCUT = 'z'
export const STUB_NAV_LABEL = 'Shortcut Stub'
// Pin a known-good bootstrap so CI is reproducible — `@latest` lets an
// unrelated bootstrap release silently turn this e2e fixture red. Bump
// deliberately when adopting a new bootstrap that changes scaffold output.
const BOOTSTRAP_VERSION = '@tinycld/bootstrap@2.4.0'

// Every subprocess this script runs finishes in well under a minute; an
// unbounded spawnSync turns a rare wedge (seen once in CI: `pnpm run
// packages:generate` produced no output and sat until the job was
// cancelled 68 minutes later) into a silent runner burn. A bounded call
// fails loudly with the step name instead.
const SUBPROCESS_TIMEOUT_MS = 5 * 60_000

function workspaceRoot(): string {
    // tinycld/tests/scripts/scaffold-shortcut-stub.ts → tinycld/ → workspace root
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
    // present) and run pnpm install. The caller is responsible for the
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
        { stdio: 'inherit', cwd: wsRoot, timeout: SUBPROCESS_TIMEOUT_MS }
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
        // works — the generator bundles it on \`pnpm install\`. We pick
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
    // Three panels sharing ONE slug, on purpose. \`settings\` is org-scoped,
    // \`accountSettings\` is per-user and \`systemSettings\` is deployment-wide,
    // and the settings area addresses them through separate route trees
    // precisely so the same slug can appear in each without one shadowing
    // another. Mail ships exactly this shape
    // (its 'provider' slug is both a domains screen and the provider picker),
    // but app-shell CI installs no feature package — so the stub carries the
    // collision, and the settings e2e proves the routing against it.
    settings: [{ slug: 'panel', label: 'Stub Org Panel', component: 'settings/panel' }],
    accountSettings: [
        { slug: 'panel', label: 'Stub Account Panel', component: 'account-settings/panel' },
    ],
    systemSettings: [
        { slug: 'panel', label: 'Stub System Panel', component: 'system-settings/panel' },
    ],
    // Core's suite installs no real package, so the stub declares the one
    // notify event the notify-registry spec drives through the toast.
    notifications: { events: 'notify-events' },
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
        './settings/*': `./tinycld/${STUB_SLUG}/settings/*.tsx`,
        './account-settings/*': `./tinycld/${STUB_SLUG}/account-settings/*.tsx`,
        './system-settings/*': `./tinycld/${STUB_SLUG}/system-settings/*.tsx`,
        './notify-events': `./tinycld/${STUB_SLUG}/notify-events.ts`,
    }
    writeFileSync(path, `${JSON.stringify(pkg, null, 4)}\n`)
}

// The three panels the manifest declares under one shared slug. Each renders a
// distinct testID so the settings e2e can prove which route tree resolved it.
function writePanels(stubDir: string): void {
    const panels = [
        { dir: 'settings', component: 'StubOrgPanel', testID: 'stub-org-panel' },
        { dir: 'account-settings', component: 'StubAccountPanel', testID: 'stub-account-panel' },
        { dir: 'system-settings', component: 'StubSystemPanel', testID: 'stub-system-panel' },
    ]
    for (const panel of panels) {
        const dir = join(stubDir, 'tinycld', STUB_SLUG, panel.dir)
        mkdirSync(dir, { recursive: true })
        writeFileSync(
            join(dir, 'panel.tsx'),
            `import { Text, View } from 'react-native'

export default function ${panel.component}() {
    return (
        <View className="flex-1 p-4 bg-background">
            <Text className="text-foreground" testID="${panel.testID}">
                ${panel.component}
            </Text>
        </View>
    )
}
`
        )
    }
}

// The events module the manifest's `notifications.events` names: data only, in
// the shape every package's module has, so the generator's checks apply to it.
function writeNotifyEvents(stubDir: string): void {
    writeFileSync(
        join(stubDir, 'tinycld', STUB_SLUG, 'notify-events.ts'),
        `import type { PackageNotifyEvents } from '@tinycld/core/lib/notify/registry'

declare module '@tinycld/core/lib/notify/events' {
    interface NotificationEvents {
        '${STUB_SLUG}.pinged': { at: string }
    }
}

const notifyEvents = {
    '${STUB_SLUG}.pinged': { channels: ['toast'], variant: 'info' },
} satisfies PackageNotifyEvents

export default notifyEvents
`
    )
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
    // practice the spec just waits for the URL change. The ping button emits
    // the stub's own notify event, so the notify-registry spec can prove a
    // package's declared event reaches the toast without naming a real package.
    writeFileSync(
        join(dir, 'index.tsx'),
        `import { notify } from '@tinycld/core/lib/notify'
import { appHref } from '@tinycld/core/lib/org-routes'
import { router } from 'expo-router'
import { Pressable, Text, View } from 'react-native'

function ping() {
    notify.emit({
        event: '${STUB_SLUG}.pinged',
        title: 'Pinged',
        data: { at: new Date().toISOString() },
    })
}

function openGuard() {
    router.push(appHref('${STUB_SLUG}/guard'))
}

export default function StubIndex() {
    return (
        <View className="flex-1 p-4 bg-background">
            <Text className="text-foreground" testID="shortcut-stub-landing">
                Shortcut stub landing
            </Text>
            <Pressable onPress={ping} accessibilityRole="button" testID="shortcut-stub-ping">
                <Text className="text-foreground">Ping</Text>
            </Pressable>
            <Pressable onPress={openGuard} accessibilityRole="button" testID="stub-open-guard">
                <Text className="text-foreground">Open guarded stack</Text>
            </Pressable>
        </View>
    )
}
`
    )
    writeGuardStack(dir)
}

// A child Stack under the stub's root navigator, whose editor is guarded by
// core useLeaveGuard. The editor's Leave button replaces to the stub's index,
// a route of the PARENT navigator, so the leave-guard-replay spec proves that
// Discard replays a parent navigator's move held on a nested screen.
function writeGuardStack(screensDir: string): void {
    const dir = join(screensDir, 'guard')
    mkdirSync(dir, { recursive: true })
    writeFileSync(
        join(dir, '_layout.tsx'),
        `import { Stack } from 'expo-router'

export default function StubGuardLayout() {
    return <Stack screenOptions={{ headerShown: false }} />
}
`
    )
    writeFileSync(
        join(dir, 'index.tsx'),
        `import { appHref } from '@tinycld/core/lib/org-routes'
import { router } from 'expo-router'
import { Pressable, Text, View } from 'react-native'

function openEditor() {
    router.push(appHref('${STUB_SLUG}/guard/edit'))
}

export default function StubGuardIndex() {
    return (
        <View className="flex-1 p-4 bg-background" testID="stub-guard-index">
            <Pressable onPress={openEditor} accessibilityRole="button" testID="stub-guard-open-editor">
                <Text className="text-foreground">Open editor</Text>
            </Pressable>
        </View>
    )
}
`
    )
    writeFileSync(
        join(dir, 'edit.tsx'),
        `import { useLeaveGuard } from '@tinycld/core/components/forms-builder/use-leave-guard'
import { appHref } from '@tinycld/core/lib/org-routes'
import { ConfirmDialog } from '@tinycld/core/ui/ConfirmDialog'
import { router } from 'expo-router'
import { useState } from 'react'
import { Pressable, Text, TextInput, View } from 'react-native'

function leaveToLanding() {
    router.replace(appHref('${STUB_SLUG}'))
}

export default function StubGuardEdit() {
    // A test fixture's one draft field: local UI state, no validation.
    const [draft, setDraft] = useState('')
    const leave = useLeaveGuard(draft !== '')
    return (
        <View className="flex-1 p-4 bg-background" testID="stub-guard-editor">
            <TextInput
                className="text-foreground"
                value={draft}
                onChangeText={setDraft}
                placeholder="Draft"
                testID="stub-guard-input"
            />
            <Pressable onPress={leaveToLanding} accessibilityRole="button" testID="stub-guard-leave">
                <Text className="text-foreground">Leave</Text>
            </Pressable>
            <ConfirmDialog
                isOpen={leave.isConfirming}
                onClose={leave.cancel}
                onConfirm={leave.discard}
                title="Discard changes?"
                message="Your draft will be lost."
                confirmLabel="Discard"
                cancelLabel="Keep editing"
                isDestructive
                requiresServer={false}
                confirmTestID="stub-leave-discard"
            />
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
    const alreadyMember = workspaces.includes(STUB_SLUG)
    if (!alreadyMember) {
        pkg.workspaces = [...workspaces, STUB_SLUG]
        writeFileSync(path, `${JSON.stringify(pkg, null, 4)}\n`)
    }
    // pnpm discovers members from pnpm-workspace.yaml, so register the stub
    // there too (the package.json workspaces[] is only a tooling hint).
    const added = ensurePnpmMember(join(wsRoot, 'pnpm-workspace.yaml'))
    if (alreadyMember && !added) return
    // pnpm install relinks workspaces. We piggy-back on the caller's install
    // (CI runs pnpm install before invoking this script) — the first-time
    // scaffold path needs a second install to wire the symlink; subsequent
    // runs are no-ops. corepack enable selects the pinned pnpm.
    // COREPACK_ENABLE_DOWNLOAD_PROMPT=0: corepack's interactive "download
    // pnpm?" confirmation is a silent stdin-wait in any non-TTY context that
    // still looks TTY-ish — one of the ways this step can hang without output.
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
        throw new Error(
            'pnpm install (post-member-add) failed; the shortcut-stub symlink under node_modules may be missing'
        )
    }
}

// Add `  - <STUB_SLUG>` to pnpm-workspace.yaml's packages: block if absent.
// Returns true if a line was added. A missing file throws (a scaffolded
// workspace always has one).
function ensurePnpmMember(yamlPath: string): boolean {
    const yaml = readFileSync(yamlPath, 'utf8')
    const lines = yaml.split('\n')
    const pkgIdx = lines.findIndex(l => /^packages:\s*$/.test(l))
    if (pkgIdx === -1) throw new Error(`no packages: block in ${yamlPath}`)
    let lastEntry = pkgIdx
    for (let i = pkgIdx + 1; i < lines.length; i++) {
        const line = lines[i] ?? ''
        if (line.trim() === `- ${STUB_SLUG}`) return false // already present
        if (/^\s+-\s+/.test(line)) lastEntry = i
        else if (/^\S/.test(line) && line.trim() !== '') break
    }
    lines.splice(lastEntry + 1, 0, `  - ${STUB_SLUG}`)
    writeFileSync(yamlPath, lines.join('\n'))
    return true
}

function regenerateConfig(wsRoot: string): void {
    // The generator emits app/tinycld.config.ts from getPackages() output.
    // After adding shortcut-stub to the workspace tree we have to re-run
    // it, otherwise the rail won't render the stub's nav entry and the
    // shortcut binding won't be registered.
    const r = spawnSync('pnpm', ['run', 'packages:generate'], {
        cwd: join(wsRoot, 'tinycld'),
        stdio: 'inherit',
        timeout: SUBPROCESS_TIMEOUT_MS,
    })
    if (r.status !== 0) {
        const cause = r.error ? ` (${r.error.message})` : ''
        throw new Error(
            `packages:generate failed${cause}; shortcut-stub will not be loaded by the app`
        )
    }
}

function main(): void {
    const wsRoot = workspaceRoot()
    console.log(`[scaffold-shortcut-stub] workspace root: ${wsRoot}`)

    ensureBootstrapped(wsRoot)

    const stubDir = join(wsRoot, STUB_SLUG)
    patchManifest(stubDir)
    patchPackageJson(stubDir)
    patchVitestConfig(stubDir)
    patchTsconfig(stubDir)
    writeScreens(stubDir)
    writePanels(stubDir)
    writeNotifyEvents(stubDir)

    ensureMember(wsRoot)
    regenerateConfig(wsRoot)

    console.log(`[scaffold-shortcut-stub] done — ${STUB_SLUG}/ ready at ${stubDir}`)
}

main()
