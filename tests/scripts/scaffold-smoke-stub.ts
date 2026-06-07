#!/usr/bin/env tsx
/**
 * scaffold-smoke-stub.ts — provisions the @tinycld/smoke-stub feature package
 * into the workspace hosting this repo, using bootstrap's `--new --preset full`
 * flow with NO patching.
 *
 * Why a stub: the generator smoke test (scripts/__tests__/generate.smoke.test.ts)
 * needs a real feature package present so it can verify the generator emits that
 * package's routes / config entry / uniwind source / help. Asserting against a
 * real first-party package (contacts, mail, …) only works in a FULL local
 * checkout — CI assembles app+core only, where those siblings are absent, so the
 * test went red. Hard-coding a checked-in stub package would drift from whatever
 * shape bootstrap actually produces. Instead we let BOOTSTRAP scaffold a
 * complete feature (the `full` preset already ships routes + nav + screens +
 * a screens/* exports entry — exactly what the generator consumes) so the
 * fixture can never diverge from bootstrap's output.
 *
 * Idempotent: if smoke-stub/ already exists (a developer ran the test once
 * locally, or the full local workspace has features), the bootstrap call is
 * skipped. The smoke test's afterAll removes it again on CI.
 *
 * Invocation: the smoke test calls scaffoldSmokeStub() from beforeAll, so the
 * stub is present wherever the test runs (CI and local) without a separate CI
 * step or any checked-in package files.
 *
 * Pattern source: tests/scripts/scaffold-shortcut-stub.ts and
 * drive/tests/scripts/scaffold-share-stub.ts — same bootstrap-then-register
 * flow. This one uses --preset full and patches NOTHING.
 */

import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const SMOKE_STUB_SLUG = 'smoke-stub'
// @latest: bootstrap is the source of truth for package shape; pin nothing so a
// stale fixture can't outlive a bootstrap change.
const BOOTSTRAP_SPEC = '@tinycld/bootstrap@latest'

// tinycld/tests/scripts/scaffold-smoke-stub.ts → tinycld/ → workspace root.
function workspaceRoot(): string {
    return resolve(fileURLToPath(import.meta.url), '..', '..', '..', '..')
}

function ensureBootstrapped(wsRoot: string): string {
    const stubDir = join(wsRoot, SMOKE_STUB_SLUG)
    if (existsSync(stubDir)) {
        console.log(`[scaffold-smoke-stub] ${SMOKE_STUB_SLUG}/ exists — skipping bootstrap`)
        return stubDir
    }
    console.log(`[scaffold-smoke-stub] running bootstrap to scaffold ${SMOKE_STUB_SLUG}/`)
    // --no-link: bootstrap would otherwise try to assemble + pnpm install; the
    // caller owns the install. --preset full ships routes + nav + screens (with
    // a screens/* exports entry) so the generator has a complete feature to emit
    // with no patching.
    execFileSync(
        'npx',
        [
            '--yes',
            BOOTSTRAP_SPEC,
            '--new',
            SMOKE_STUB_SLUG,
            '--yes',
            '--preset',
            'full',
            '--name',
            'Smoke Stub',
            '--description',
            'generator smoke-test fixture (scaffolded by bootstrap)',
            '--no-link',
            '--target',
            stubDir,
        ],
        { stdio: 'inherit', cwd: wsRoot }
    )
    return stubDir
}

// Register the stub as a workspace member so getPackages() (and thus the
// generator) discovers it. pnpm discovers members from pnpm-workspace.yaml; the
// package.json workspaces[] is only a tooling hint. getPackages() scans the
// workspace root for member dirs with a manifest.ts and does NOT require the
// pnpm symlink, so no reinstall is needed for the generator to see the stub.
function ensureMember(wsRoot: string): void {
    ensureWorkspacesHint(join(wsRoot, 'package.json'))
    ensurePnpmMember(join(wsRoot, 'pnpm-workspace.yaml'))
}

function ensureWorkspacesHint(pkgPath: string): void {
    if (!existsSync(pkgPath)) return
    const pkg = JSON.parse(readFileSync(pkgPath, 'utf8'))
    const workspaces: string[] = Array.isArray(pkg.workspaces) ? pkg.workspaces : []
    if (workspaces.includes(SMOKE_STUB_SLUG)) return
    pkg.workspaces = [...workspaces, SMOKE_STUB_SLUG]
    writeFileSync(pkgPath, `${JSON.stringify(pkg, null, 4)}\n`)
}

// Add `  - smoke-stub` to pnpm-workspace.yaml's packages: block if absent.
function ensurePnpmMember(yamlPath: string): void {
    if (!existsSync(yamlPath)) return
    const lines = readFileSync(yamlPath, 'utf8').split('\n')
    const pkgIdx = lines.findIndex(l => /^packages:\s*$/.test(l))
    if (pkgIdx === -1) return
    let lastEntry = pkgIdx
    for (let i = pkgIdx + 1; i < lines.length; i++) {
        const line = lines[i] ?? ''
        if (line.trim() === `- ${SMOKE_STUB_SLUG}`) return // already present
        if (/^\s+-\s+/.test(line)) lastEntry = i
        else if (/^\S/.test(line) && line.trim() !== '') break
    }
    lines.splice(lastEntry + 1, 0, `  - ${SMOKE_STUB_SLUG}`)
    writeFileSync(yamlPath, lines.join('\n'))
}

/** Scaffold + register smoke-stub. Returns its directory. Idempotent. */
export function scaffoldSmokeStub(): string {
    const wsRoot = workspaceRoot()
    const stubDir = ensureBootstrapped(wsRoot)
    ensureMember(wsRoot)
    return stubDir
}

// Remove the stub member registration (used by the smoke test's cleanup). The
// stub DIRECTORY removal is the caller's responsibility — a developer's full
// local workspace may legitimately keep it.
export function unregisterSmokeStub(): void {
    const wsRoot = workspaceRoot()
    const pkgPath = join(wsRoot, 'package.json')
    if (existsSync(pkgPath)) {
        const pkg = JSON.parse(readFileSync(pkgPath, 'utf8'))
        if (Array.isArray(pkg.workspaces)) {
            pkg.workspaces = pkg.workspaces.filter((m: string) => m !== SMOKE_STUB_SLUG)
            writeFileSync(pkgPath, `${JSON.stringify(pkg, null, 4)}\n`)
        }
    }
    const yamlPath = join(wsRoot, 'pnpm-workspace.yaml')
    if (existsSync(yamlPath)) {
        const lines = readFileSync(yamlPath, 'utf8').split('\n')
        const kept = lines.filter(l => l.trim() !== `- ${SMOKE_STUB_SLUG}`)
        writeFileSync(yamlPath, kept.join('\n'))
    }
}

// Allow running standalone (e.g. a CI step) as well as importing the functions.
if (resolve(fileURLToPath(import.meta.url)) === resolve(process.argv[1] ?? '')) {
    scaffoldSmokeStub()
}
