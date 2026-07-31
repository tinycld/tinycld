import http from 'node:http'
import { expect, test } from '@playwright/test'

// Integration test for the MULTI-ORG package-deploy flow, driven over real
// HTTP against a running serve-multi router (the runner script
// `tests/install/run-multiorg-deploy.sh` builds and boots it). This is the
// multi-org sibling of todo-install.spec.ts, covering the slice of the
// package lifecycle that exists in the hosted model TODAY:
//
//   superuser publish to the store → create an org from a lockfile →
//   deploy a new version → the respawned tenant serves the new package
//   and its migration ran on boot.
//
// It deliberately does NOT cover the in-app Packages UI, downgrade with
// down-migrations, build-history rollback, or uninstall — none of those have
// a multi-org counterpart yet. Per multi-org/docs/DESIGN-org-package-agency.md
// (§7), steps 2–4 give hosted tenants the same in-app package flow the
// single-tenant app has; when step 4 lands, todo-install.spec.ts itself
// becomes runnable against an org subdomain and is the acceptance test for
// that work. Until then THIS spec pins the router-side operator flow — which
// no other test exercises over the wire (the Go suites call the Provisioner
// in-process and never bind MT_ADDR or exercise RequireSuperuserAuth).
//
// The fixture mirrors the github:tinycld/todo contract shape (todos in v1,
// tags/todo_tags added in v2) but is TS-only and inline: the hosted store
// cannot run the real todo package's Go server (pinned tenant menu) and
// nothing serves a client bundle yet, so a local publishable fixture keeps
// the test hermetic — no network fetch, no github dependency.
//
// Networking: the router dispatches purely on the Host header's subdomain
// (multi-org/internal/frontrouter). API calls here use node:http against
// 127.0.0.1 with an explicit Host header — deterministic on every platform,
// no DNS. Browser navigations use *.localhost URLs, which Chromium resolves
// to loopback natively (run with PW_MULTIORG_BASE_DOMAIN=localhost; any other
// base domain needs /etc/hosts entries for the browser tests).
//
// Deliberately self-contained — shares no fixtures with todo-install or e2e,
// matching the tests/install convention (see calendar-slots-install.spec.ts).

const RUN = process.env.RUN_MULTIORG_DEPLOY_TEST === '1'

const MT_URL = new URL(process.env.PW_MULTIORG_URL ?? 'http://127.0.0.1:7091')
const BASE_DOMAIN = process.env.PW_MULTIORG_BASE_DOMAIN ?? 'localhost'
const SUPERUSER_EMAIL = process.env.MT_SUPERUSER_EMAIL || 'multiorg-smoke@example.com'
const SUPERUSER_PASSWORD = process.env.MT_SUPERUSER_PASSWORD || 'MultiOrgSmoke1234!'

const ORG_SLUG = 'acme'
const PKG_NAME = '@tinycld/todo'

// How long the tenant may take to (re)spawn before a probe attempt counts as
// a failure. CreateOrg's own verify allows 60s; deploy+evict respawn is
// lazier (first request pays the cold start), so poll generously.
const TENANT_READY_MS = 90_000

type ApiResponse = { status: number; body: string; json: () => unknown }

// apiRequest speaks to the router at 127.0.0.1:<port> with an explicit Host
// header selecting the subdomain ('' = apex, 'admin' = control plane,
// '<slug>' = that org's tenant). node:http rather than fetch: undici treats
// Host as a forbidden header, and OS resolvers disagree about *.localhost —
// this path has neither problem.
function apiRequest(
    subdomain: string,
    method: string,
    path: string,
    opts: { token?: string; body?: unknown } = {}
): Promise<ApiResponse> {
    const host = subdomain === '' ? BASE_DOMAIN : `${subdomain}.${BASE_DOMAIN}`
    const payload = opts.body === undefined ? undefined : JSON.stringify(opts.body)
    const headers: Record<string, string> = { Host: `${host}:${MT_URL.port}` }
    if (opts.token) headers.Authorization = opts.token
    if (payload) headers['Content-Type'] = 'application/json'
    return new Promise((resolve, reject) => {
        const req = http.request(
            { host: MT_URL.hostname, port: MT_URL.port, method, path, headers },
            (res) => {
                let body = ''
                res.setEncoding('utf8')
                res.on('data', (chunk) => {
                    body += chunk
                })
                res.on('end', () =>
                    resolve({
                        status: res.statusCode ?? 0,
                        body,
                        json: () => JSON.parse(body) as unknown,
                    })
                )
            }
        )
        req.on('error', reject)
        if (payload) req.write(payload)
        req.end()
    })
}

async function superuserToken(): Promise<string> {
    const res = await apiRequest('admin', 'POST', '/api/collections/_superusers/auth-with-password', {
        body: { identity: SUPERUSER_EMAIL, password: SUPERUSER_PASSWORD },
    })
    expect(res.status, `superuser auth failed: ${res.body}`).toBe(200)
    return (res.json() as { token: string }).token
}

// ---------- the publishable todo fixture ----------
//
// Mirrors the schema contract of the github:tinycld/todo fixture the
// single-tenant test installs: v1 ships only the todos collection; v2 adds
// tags + todo_tags. The probe hook reports its own version and which
// collections exist, so the spec can assert both "the new hook code is live"
// and "the new migration ran on the respawned tenant's boot" through the
// tenant's public surface (its DB and collections API need auth; the hook
// runs inside the tenant and needs none).
//
// Store constraints honored here: manifest.ts MUST use `export default`
// (eval'd as CJS at publish); pb-hooks/pb-migrations run as SCRIPTS in the
// tenant jsvm, so they must be module-syntax-free; .ts files are transpiled
// and renamed .ts → .js at publish. v2 ships BOTH migrations — a store
// version is the whole package, and the tenant's boot-time RunAllMigrations
// skips the already-applied one.

function manifestTs(version: string): string {
    return [
        'const manifest = {',
        `    name: '${PKG_NAME}',`,
        "    slug: 'todo',",
        `    version: '${version}',`,
        "    description: 'Todo fixture for the multi-org deploy e2e',",
        '}',
        'export default manifest',
        '',
    ].join('\n')
}

const CREATE_TODOS_MIGRATION = [
    'migrate((app) => {',
    '    const todos = new Collection({',
    "        id: 'pbc_e2e_todos_01',",
    "        name: 'todos',",
    "        type: 'base',",
    "        fields: [{ id: 'todo_text', name: 'text', type: 'text' }],",
    '    } as any)',
    '    app.save(todos)',
    '}, (app) => {',
    "    app.delete(app.findCollectionByNameOrId('todos'))",
    '})',
    '',
].join('\n')

const CREATE_TAGS_MIGRATION = [
    'migrate((app) => {',
    '    const tags = new Collection({',
    "        id: 'pbc_e2e_tags_01',",
    "        name: 'tags',",
    "        type: 'base',",
    "        fields: [{ id: 'tag_label', name: 'label', type: 'text' }],",
    '    } as any)',
    '    app.save(tags)',
    '    const todoTags = new Collection({',
    "        id: 'pbc_e2e_todo_tags_01',",
    "        name: 'todo_tags',",
    "        type: 'base',",
    '        fields: [',
    "            { id: 'tt_todo', name: 'todo', type: 'text' },",
    "            { id: 'tt_tag', name: 'tag', type: 'text' },",
    '        ],',
    '    } as any)',
    '    app.save(todoTags)',
    '}, (app) => {',
    "    app.delete(app.findCollectionByNameOrId('todo_tags'))",
    "    app.delete(app.findCollectionByNameOrId('tags'))",
    '})',
    '',
].join('\n')

function probeHookTs(version: string): string {
    return [
        "routerAdd('GET', '/api/todo-probe', (e) => {",
        '    const has = (name: string) => {',
        '        try {',
        '            $app.findCollectionByNameOrId(name)',
        '            return true',
        '        } catch (err) {',
        '            return false',
        '        }',
        '    }',
        `    return e.json(200, { version: '${version}', todos: has('todos'), tags: has('tags') })`,
        '})',
        '',
    ].join('\n')
}

function b64(content: string): string {
    return Buffer.from(content, 'utf8').toString('base64')
}

function fixtureFiles(version: string): Record<string, string> {
    const files: Record<string, string> = {
        'manifest.ts': b64(manifestTs(version)),
        'pb-migrations/1700000001_create_todos.ts': b64(CREATE_TODOS_MIGRATION),
        'pb-hooks/todo-probe.pb.ts': b64(probeHookTs(version)),
    }
    if (version !== '1.0.0') {
        files['pb-migrations/1700000002_create_tags.ts'] = b64(CREATE_TAGS_MIGRATION)
    }
    return files
}

type Probe = { version: string; todos: boolean; tags: boolean }

// pollProbe retries the tenant's probe route until it reports the expected
// package version (or the deadline passes). Errors and stale versions are
// both transient: a deploy evicts the tenant, and the next request pays the
// cold start (spawn + RunAllMigrations + jsvm compile).
async function pollProbe(expectVersion: string): Promise<Probe> {
    const deadline = Date.now() + TENANT_READY_MS
    let last = ''
    for (;;) {
        try {
            const res = await apiRequest(ORG_SLUG, 'GET', '/api/todo-probe')
            last = `${res.status} ${res.body.slice(0, 200)}`
            if (res.status === 200) {
                const probe = res.json() as Probe
                if (probe.version === expectVersion) return probe
            }
        } catch (err) {
            last = String(err)
        }
        if (Date.now() > deadline) {
            throw new Error(`tenant never served todo-probe v${expectVersion}; last response: ${last}`)
        }
        await new Promise((r) => setTimeout(r, 1500))
    }
}

test.describe.serial('multi-org package deploy', () => {
    test.skip(!RUN, 'multi-org deploy test only runs via tests/install/run-multiorg-deploy.sh (RUN_MULTIORG_DEPLOY_TEST=1)')

    let token = ''

    test('superuser authenticates against the control plane over HTTP', async () => {
        token = await superuserToken()
        expect(token).not.toBe('')
    })

    test('publishes todo v1 + v2; published versions are immutable', async () => {
        for (const version of ['1.0.0', '2.0.0']) {
            const res = await apiRequest('admin', 'POST', '/api/store/packages', {
                token,
                body: { name: PKG_NAME, version, kind: 'official', files: fixtureFiles(version) },
            })
            expect(res.status, `publish ${version} failed: ${res.body}`).toBe(200)
        }
        // The store refuses to overwrite a published version — the immutability
        // the shared-store trust model depends on.
        const repub = await apiRequest('admin', 'POST', '/api/store/packages', {
            token,
            body: { name: PKG_NAME, version: '1.0.0', kind: 'official', files: fixtureFiles('1.0.0') },
        })
        expect(repub.status, 'republish of an existing version must be refused').toBe(400)
    })

    test('creates an org from a lockfile; the tenant serves the package', async () => {
        // CreateOrg materializes, boots the real tenant, and verifies readiness
        // before returning — budget for the cold start.
        test.setTimeout(TENANT_READY_MS + 60_000)
        const res = await apiRequest('admin', 'POST', '/api/orgs', {
            token,
            body: { slug: ORG_SLUG, display_name: 'Acme', lockfile: { [PKG_NAME]: '1.0.0' } },
        })
        expect(res.status, `create org failed: ${res.body}`).toBe(200)
        expect(res.json()).toMatchObject({ slug: ORG_SLUG, status: 'active' })

        const health = await apiRequest(ORG_SLUG, 'GET', '/api/health')
        expect(health.status, `tenant health: ${health.body}`).toBe(200)

        const probe = await pollProbe('1.0.0')
        expect(probe.todos, 'v1 migration must have created todos on the provisioning boot').toBe(true)
        expect(probe.tags, 'v1 must not ship the tags schema').toBe(false)
    })

    test('deploys v2; the respawned tenant serves the new version and its migration ran', async () => {
        test.setTimeout(TENANT_READY_MS + 60_000)
        const res = await apiRequest('admin', 'POST', `/api/orgs/${ORG_SLUG}/deploy`, {
            token,
            body: { lockfile: { [PKG_NAME]: '2.0.0' } },
        })
        expect(res.status, `deploy failed: ${res.body}`).toBe(204)

        // Deploy re-materializes and evicts; the next request respawns the
        // tenant, whose boot applies the new create_tags migration.
        const probe = await pollProbe('2.0.0')
        expect(probe.todos, 'existing todos schema must survive the upgrade').toBe(true)
        expect(probe.tags, 'v2 migration must run on the respawned tenant boot').toBe(true)
    })

    test('the deploy is recorded in the deployments audit table', async () => {
        const res = await apiRequest(
            'admin',
            'GET',
            '/api/collections/deployments/records?perPage=10',
            { token }
        )
        expect(res.status, res.body).toBe(200)
        const list = res.json() as { totalItems: number; items: Array<{ lockfile: Record<string, string> }> }
        expect(list.totalItems, 'exactly the one explicit deploy is audited (CreateOrg is not a deploy)').toBe(1)
        expect(list.items[0].lockfile).toMatchObject({ [PKG_NAME]: '2.0.0' })
    })

    test('an unresolvable deploy is refused and the org keeps serving v2', async () => {
        const res = await apiRequest('admin', 'POST', `/api/orgs/${ORG_SLUG}/deploy`, {
            token,
            body: { lockfile: { [PKG_NAME]: '9.9.9' } },
        })
        expect(res.status, 'deploying a version absent from the store must 400').toBe(400)

        const probe = await pollProbe('2.0.0')
        expect(probe.tags).toBe(true)
    })

    test('provisioning routes reject unauthenticated callers', async () => {
        const res = await apiRequest('admin', 'POST', `/api/orgs/${ORG_SLUG}/deploy`, {
            body: { lockfile: { [PKG_NAME]: '2.0.0' } },
        })
        expect([401, 403], `unauthenticated deploy must be refused, got ${res.status}`).toContain(res.status)
    })

    test('the router serves the apex org-finder and unknown-org pages to a browser', async ({ page }) => {
        // Chromium resolves *.localhost to loopback natively — these are real
        // browser navigations through the front router's Host dispatch.
        await page.goto(`http://${BASE_DOMAIN}:${MT_URL.port}/`)
        await expect(page.getByText('Find your organization')).toBeVisible()

        const missing = await page.goto(`http://no-such-org.${BASE_DOMAIN}:${MT_URL.port}/`)
        expect(missing?.status()).toBe(404)
        await expect(page.getByText('Organization not found')).toBeVisible()
    })
})
