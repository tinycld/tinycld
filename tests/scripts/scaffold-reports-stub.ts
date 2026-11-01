#!/usr/bin/env tsx
/**
 * scaffold-reports-stub.ts — provisions the @tinycld/salesstub package into the
 * workspace that contains the app shell.
 *
 * Why a stub: the reports hub, view and builder read report sources, which only
 * packages register, and core must not name a package. App-shell CI installs no
 * feature package, so without a stub the reports e2e (tests/e2e/reports-*.spec.ts)
 * has nothing to report on. The stub registers two sources from Go (a to-one and
 * a to-many link, money with a currency field, a number, a date and a select) over
 * its own collections and one shipped report with a formula the builder cannot
 * edit, and its landing screen adds rows through the UI, as the specs must.
 *
 * Writes every file itself, as scaffold-forms-host-stub.ts does: no bootstrap
 * preset ships a migration, a Go module and collections together. The screen,
 * the migration and the Go source are kept as .txt templates in
 * tests/scripts/reports-stub/ and copied as they are. The Go module pins the Go and
 * PocketBase versions core's go.mod uses, as scaffold-search-stubs.ts explains.
 *
 * Invocation mirrors the other scaffolders:
 *   - CI: `pnpm exec tsx tinycld/tests/scripts/scaffold-reports-stub.ts` from the
 *     workspace root, after `pnpm install` and before the bundle build.
 *   - Local: run once before `tinycld-pkg test:e2e`.
 *
 * Idempotent: re-running re-emits the sources and installs only when the stub is
 * not yet a workspace member.
 *
 * Fixture names the specs assert on are listed in tests/e2e/reports-helpers.ts.
 */

import { spawnSync } from 'node:child_process'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const STUB_DIR = 'salesstub'
export const STUB_SLUG = 'salesstub'
export const STUB_PACKAGE = '@tinycld/salesstub'
const TEMPLATES_DIR = resolve(fileURLToPath(import.meta.url), '..', 'reports-stub')
const TEMPLATES: [string, string][] = [
    ['screens/index.tsx.txt', `tinycld/${STUB_SLUG}/screens/index.tsx`],
    ['pb-migrations/2090000300_salesstub.js.txt', 'pb-migrations/2090000300_salesstub.js'],
    ['server/register.go.txt', 'server/register.go'],
]
const SUBPROCESS_TIMEOUT_MS = 5 * 60_000

function workspaceRoot(): string {
    return resolve(fileURLToPath(import.meta.url), '..', '..', '..', '..')
}

function write(path: string, contents: string): void {
    mkdirSync(resolve(path, '..'), { recursive: true })
    writeFileSync(path, contents)
}

function goVersions(wsRoot: string): { go: string; pocketbase: string } {
    const modPath = join(wsRoot, 'tinycld', 'core', 'server', 'go.mod')
    const mod = readFileSync(modPath, 'utf8')
    const go = mod.match(/^go\s+(\S+)/m)?.[1]
    const pocketbase = mod.match(/github\.com\/pocketbase\/pocketbase\s+(v\S+)/)?.[1]
    if (!go || !pocketbase) throw new Error(`could not read go/pocketbase versions from ${modPath}`)
    return { go, pocketbase }
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
            './collections': `${src}/collections.ts`,
            './types': `${src}/types.ts`,
            './screens/*': `${src}/screens/*.tsx`,
        },
        peerDependencies: {},
    }
    write(join(stubDir, 'package.json'), `${JSON.stringify(pkg, null, 4)}\n`)
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
    "exclude": ["node_modules", "pb-migrations", "server"]
}
`
    )
}

function writeManifest(stubDir: string): void {
    write(
        join(stubDir, 'manifest.ts'),
        `const manifest = {
    name: 'Sales Stub',
    slug: '${STUB_SLUG}',
    version: '0.1.0',
    description: 'app E2E reports stub',
    routes: { directory: 'screens' },
    nav: { label: 'Sales Stub', icon: 'flask-conical', order: 997 },
    migrations: { directory: 'pb-migrations' },
    collections: { register: 'collections', types: 'types' },
    server: { package: 'server', module: 'tinycld.org/packages/${STUB_SLUG}' },
}

export default manifest
`
    )
}

function writeSources(stubDir: string): void {
    const dir = join(stubDir, 'tinycld', STUB_SLUG)
    write(
        join(dir, 'collections.ts'),
        `import type { Schema } from '@tinycld/core/types/pbSchema'
import type { createCollection } from 'pbtsdb/core'

const onDemand = { syncMode: 'on-demand', realtime: 'query' } as const

export function registerCollections(newCollection: ReturnType<typeof createCollection<Schema>>) {
    const salesstub_teams = newCollection('salesstub_teams', {
        omitOnInsert: ['created', 'updated'] as const,
        ...onDemand,
    })
    const salesstub_sales = newCollection('salesstub_sales', {
        omitOnInsert: ['created', 'updated'] as const,
        ...onDemand,
    })
    return { salesstub_teams, salesstub_sales }
}
`
    )
    write(
        join(dir, 'types.ts'),
        `import type { Schema } from '@tinycld/core/types/pbSchema'

export type SalesstubSchema = Pick<Schema, 'salesstub_teams' | 'salesstub_sales'>
`
    )
}

function writeGoModule(stubDir: string, versions: { go: string; pocketbase: string }): void {
    write(
        join(stubDir, 'server', 'go.mod'),
        `module tinycld.org/packages/${STUB_SLUG}

go ${versions.go}

require (
	github.com/pocketbase/pocketbase ${versions.pocketbase}
	tinycld.org/core v0.0.0
)
`
    )
}

function copyTemplates(stubDir: string): void {
    for (const [from, to] of TEMPLATES)
        write(join(stubDir, to), readFileSync(join(TEMPLATES_DIR, from), 'utf8'))
}

function ensureMember(wsRoot: string): boolean {
    const path = join(wsRoot, 'package.json')
    const pkg = JSON.parse(readFileSync(path, 'utf8'))
    const workspaces: string[] = Array.isArray(pkg.workspaces) ? pkg.workspaces : []
    const missing = !workspaces.includes(STUB_DIR)
    if (missing) {
        pkg.workspaces = [...workspaces, STUB_DIR]
        writeFileSync(path, `${JSON.stringify(pkg, null, 4)}\n`)
    }
    return ensurePnpmMember(join(wsRoot, 'pnpm-workspace.yaml')) || missing
}

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

function run(cmd: string, args: string[], cwd: string, what: string): void {
    const env = { ...process.env, COREPACK_ENABLE_DOWNLOAD_PROMPT: '0' }
    const r = spawnSync(cmd, args, { cwd, stdio: 'inherit', timeout: SUBPROCESS_TIMEOUT_MS, env })
    if (r.status !== 0) {
        const cause = r.error ? `: ${r.error.message}` : ''
        throw new Error(
            `${what} failed (status ${r.status})${cause}; the reports stub will not load`
        )
    }
}

function main(): void {
    const wsRoot = workspaceRoot()
    console.log(`[scaffold-reports-stub] workspace root: ${wsRoot}`)
    const stubDir = join(wsRoot, STUB_DIR)
    writePackage(stubDir)
    writeManifest(stubDir)
    writeSources(stubDir)
    writeGoModule(stubDir, goVersions(wsRoot))
    copyTemplates(stubDir)
    if (ensureMember(wsRoot)) {
        spawnSync('corepack', ['enable'], {
            cwd: wsRoot,
            stdio: 'inherit',
            timeout: SUBPROCESS_TIMEOUT_MS,
        })
        run('pnpm', ['install', '--no-frozen-lockfile'], wsRoot, 'pnpm install')
    }
    // Links the stub's routes, migration, collections and Go module into the app.
    run('pnpm', ['run', 'packages:generate'], join(wsRoot, 'tinycld'), 'packages:generate')
    console.log(`[scaffold-reports-stub] done — ${STUB_DIR}/ ready at ${stubDir}`)
}

main()
