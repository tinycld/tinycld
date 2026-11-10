#!/usr/bin/env tsx
/**
 * scaffold-forms-host-stub.ts — provisions the @tinycld/forms-host-stub
 * package into the workspace that contains the app shell.
 *
 * Why a stub: the forms builder (Settings → Forms) only appears when a
 * package declares a form host, and core must not name a package. App-shell
 * CI installs no feature package, so without a host the builder e2e
 * (tests/e2e/forms-builder.spec.ts) has nothing to edit. The stub's
 * `forms-stub:widget` host ("Widget") sits over a minimal collection, which
 * is exactly the surface the builder needs: built-in fields, a form relation
 * and a JSON column for custom values. Its `forms-stub:order` host ("Order")
 * adds a table built-in kept in a rows collection, for the renderer e2e.
 *
 * Unlike scaffold-shortcut-stub.ts this script writes every file itself
 * rather than starting from a bootstrap preset: no preset ships a migration,
 * collections or a forms host, so a preset would be replaced wholesale anyway,
 * and writing the files directly keeps the fixture free of network access and
 * of bootstrap template drift.
 *
 * Invocation mirrors scaffold-shortcut-stub.ts:
 *   - CI: `pnpm exec tsx tinycld/tests/scripts/scaffold-forms-host-stub.ts`
 *     from the workspace root, after `pnpm install` and before the bundle
 *     build, so the stub's routes and host are in the exported bundle.
 *   - Local: run once before `tinycld-pkg test:e2e`.
 *
 * Idempotent: re-running re-emits the sources, and installs only when the
 * stub is not yet a workspace member.
 *
 * Fixture identifiers the specs assert on (keep in sync with
 * tests/e2e/forms-builder.spec.ts and tests/e2e/forms-table.spec.ts): host
 * "Widget" with built-in keys `name`, `notes`, `due`; host "Order"
 * (`forms-stub:order`) with built-ins `name`, `lines` (a table: item, qty,
 * rate, amount) and `subtotal`, drawn by the OrdersPanel on the landing page.
 *
 * The Order host's sources are kept as `.txt` templates in
 * tests/scripts/forms-host-stub/ and copied as they are, so they need no
 * escaping inside this script and no tooling in this repo reads them as code.
 */

import { spawnSync } from 'node:child_process'
import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const STUB_DIR = 'forms-host-stub'
export const STUB_SLUG = 'forms-stub'
export const STUB_PACKAGE = '@tinycld/forms-host-stub'
const COLLECTION = 'forms_stub_widgets'
// Must sort after core's 2090000000_create_forms.js: the widgets collection
// holds a relation to `forms`, which has to exist when this migration runs.
const MIGRATION = '2090000100_forms_stub_widgets.js'
const ORDERS = 'forms_stub_orders'
const LINES = 'forms_stub_order_lines'
const TEMPLATES_DIR = resolve(fileURLToPath(import.meta.url), '..', 'forms-host-stub')
/** Template (under TEMPLATES_DIR) → path in the stub, copied as is. */
const TEMPLATES: [string, string][] = [
    ['forms-ui.tsx.txt', `tinycld/${STUB_SLUG}/forms-ui.tsx`],
    ['orders/line-mapper.ts.txt', `tinycld/${STUB_SLUG}/orders/line-mapper.ts`],
    ['orders/OrdersPanel.tsx.txt', `tinycld/${STUB_SLUG}/orders/OrdersPanel.tsx`],
    [
        'pb-migrations/2090000200_forms_stub_orders.js.txt',
        'pb-migrations/2090000200_forms_stub_orders.js',
    ],
]

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
            './forms': `${src}/forms.ts`,
            './forms-ui': `${src}/forms-ui.tsx`,
            './collections': `${src}/collections.ts`,
            './types': `${src}/types.ts`,
            './screens/*': `${src}/screens/*.tsx`,
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
    "exclude": ["node_modules", "pb-migrations"]
}
`
    )
}

function writeManifest(stubDir: string): void {
    write(
        join(stubDir, 'manifest.ts'),
        `const manifest = {
    name: 'Forms Stub',
    slug: '${STUB_SLUG}',
    version: '0.1.0',
    description: 'app E2E forms-host stub',
    routes: { directory: 'screens' },
    nav: { label: 'Forms Stub', icon: 'clipboard-list', order: 998 },
    migrations: { directory: 'pb-migrations' },
    collections: { register: 'collections', types: 'types' },
    forms: { hosts: 'forms', ui: 'forms-ui' },
}

export default manifest
`
    )
}

function writeMigration(stubDir: string): void {
    const id = (name: string) => `${COLLECTION}_${name}`
    write(
        join(stubDir, 'pb-migrations', MIGRATION),
        `/// <reference path="../../tinycld/server/pb_data/types.d.ts" />
// E2E fixture: the collection behind the forms-stub:widget host.
const SIGNED_IN = '@request.auth.id != ""'

migrate(
    app => {
        const forms = app.findCollectionByNameOrId('forms')
        const widgets = new Collection({
            id: 'pbc_forms_stub_widgets',
            name: '${COLLECTION}',
            type: 'base',
            system: false,
            listRule: SIGNED_IN,
            viewRule: SIGNED_IN,
            createRule: SIGNED_IN,
            updateRule: SIGNED_IN,
            deleteRule: SIGNED_IN,
            fields: [
                { id: '${id('name')}', name: 'name', type: 'text', required: true, max: 200 },
                { id: '${id('notes')}', name: 'notes', type: 'text', max: 5000 },
                { id: '${id('due')}', name: 'due', type: 'date' },
                {
                    id: '${id('form')}',
                    name: 'form',
                    type: 'relation',
                    collectionId: forms.id,
                    cascadeDelete: false,
                    maxSelect: 1,
                },
                { id: '${id('fields')}', name: 'fields', type: 'json', maxSize: 1000000 },
                {
                    id: '${id('created')}',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: '${id('updated')}',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
        })
        app.save(widgets)
    },
    app => {
        app.delete(app.findCollectionByNameOrId('${COLLECTION}'))
    }
)
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
    const ${COLLECTION} = newCollection('${COLLECTION}', {
        omitOnInsert: ['created', 'updated'] as const,
        ...onDemand,
    })
    const ${ORDERS} = newCollection('${ORDERS}', {
        omitOnInsert: ['created', 'updated'] as const,
        ...onDemand,
    })
    const ${LINES} = newCollection('${LINES}', {
        omitOnInsert: ['created', 'updated'] as const,
        ...onDemand,
    })
    return { ${COLLECTION}, ${ORDERS}, ${LINES} }
}
`
    )
    write(
        join(dir, 'types.ts'),
        `import type { Schema } from '@tinycld/core/types/pbSchema'

export type FormsStubSchema = Pick<
    Schema,
    '${COLLECTION}' | '${ORDERS}' | '${LINES}'
>
`
    )
    write(
        join(dir, 'forms.ts'),
        `import type { FormHostDefInput } from '@tinycld/core/lib/forms/host'

const hosts: FormHostDefInput[] = [
    {
        key: '${STUB_SLUG}:widget',
        label: 'Widget',
        collection: '${COLLECTION}',
        formColumn: 'form',
        fieldsColumn: 'fields',
        columns: ['name', 'notes', 'due', 'form', 'fields'],
        builtin: [
            { key: 'name', label: 'Name', kind: 'text', storage: 'column', required: true },
            { key: 'notes', label: 'Notes', kind: 'textarea', storage: 'column' },
            { key: 'due', label: 'Due', kind: 'date', storage: 'column' },
        ],
    },
    {
        key: '${STUB_SLUG}:order',
        label: 'Order',
        collection: '${ORDERS}',
        formColumn: 'form',
        fieldsColumn: 'fields',
        columns: ['name', 'subtotal', 'form', 'fields'],
        builtin: [
            { key: 'name', label: 'Name', kind: 'text', storage: 'column', required: true },
            {
                key: 'lines',
                label: 'Lines',
                kind: 'table',
                storage: 'rows',
                columns: [
                    {
                        key: 'item',
                        label: 'Item',
                        kind: 'text',
                        required: true,
                        component: 'ItemPicker',
                        flex: 3,
                    },
                    { key: 'qty', label: 'Qty', kind: 'number', minWidth: 170, flex: 0.5 },
                    { key: 'rate', label: 'Rate', kind: 'number', minWidth: 170, flex: 0.5 },
                    {
                        key: 'amount',
                        label: 'Amount',
                        kind: 'computed',
                        minWidth: 112,
                        flex: 0.5,
                        decimals: 2,
                        expression: { mul: [{ var: 'qty' }, { var: 'rate' }] },
                    },
                ],
            },
            {
                key: 'subtotal',
                label: 'Subtotal',
                kind: 'computed',
                storage: 'column',
                decimals: 2,
                expression: { sumOf: [{ var: 'lines' }, 'amount'] },
            },
        ],
    },
]

export default hosts
`
    )
    write(
        join(dir, 'screens', 'index.tsx'),
        `import { Text, View } from 'react-native'
import { OrdersPanel } from '../orders/OrdersPanel'

export default function FormsStubIndex() {
    return (
        <View className="flex-1 p-4 bg-background">
            <Text className="text-foreground" testID="forms-stub-landing">
                Forms stub landing
            </Text>
            <OrdersPanel />
        </View>
    )
}
`
    )
}

function copyTemplates(stubDir: string): void {
    for (const [from, to] of TEMPLATES)
        write(join(stubDir, to), readFileSync(join(TEMPLATES_DIR, from), 'utf8'))
    // forms-ui is a .tsx template now; a stub scaffolded before that still has the old .ts.
    rmSync(join(stubDir, 'tinycld', STUB_SLUG, 'forms-ui.ts'), { force: true })
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
        throw new Error(
            `pnpm install failed (status ${r.status}); the forms-host stub will not link`
        )
    }
}

/**
 * Regenerate so the stub's host, migration, collections and route land in the
 * generated config and server links. Without this the package exists on disk
 * and contributes nothing.
 */
function regenerateConfig(wsRoot: string): void {
    const r = spawnSync('pnpm', ['run', 'packages:generate'], {
        cwd: join(wsRoot, 'tinycld'),
        stdio: 'inherit',
        timeout: SUBPROCESS_TIMEOUT_MS,
    })
    if (r.status !== 0) {
        const cause = r.error ? `: ${r.error.message}` : ''
        throw new Error(`packages:generate failed${cause}; the forms-host stub will not be loaded`)
    }
}

function main(): void {
    const wsRoot = workspaceRoot()
    console.log(`[scaffold-forms-host-stub] workspace root: ${wsRoot}`)
    const stubDir = join(wsRoot, STUB_DIR)
    writePackage(stubDir)
    writeManifest(stubDir)
    writeMigration(stubDir)
    writeSources(stubDir)
    copyTemplates(stubDir)
    if (ensureMember(wsRoot)) install(wsRoot)
    regenerateConfig(wsRoot)
    console.log(`[scaffold-forms-host-stub] done — ${STUB_DIR}/ ready at ${stubDir}`)
}

main()
