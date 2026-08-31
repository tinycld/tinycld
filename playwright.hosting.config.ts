import path from 'node:path'
import { defineConfig, devices } from '@playwright/test'

// Playwright config for the MULTI-ORG stack: two tenant backends behind the real
// front router, reachable as acme.localhost / globex.localhost.
//
// Separate from the main config because it is a different topology (two DBs, two
// listeners, a router in front) and is opt-in: it needs `go` on PATH plus a
// bundle the ordinary e2e run has already exported. Run it with
//
//   pnpm run e2e:hosting
//
// The specs live in tests/e2e-hosting/ so the default run cannot pick them up
// — they would fail against the single-origin webServer, which serves no
// subdomains.
const PORT = Number(process.env.E2E_HOSTING_PORT ?? 7300)

// When the stack is already running — the hosting repo's TestHostedBrowserE2E
// provisions two real orgs, spawns their tenant processes and fronts them with
// the real router, then invokes this config against that — we must NOT start a
// webServer of our own. That path is the higher-fidelity one: real provisioning
// and real tenants, versus the local launcher's two plain PocketBase instances
// behind a stand-in proxy.
const EXTERNAL = process.env.E2E_HOSTING_EXTERNAL === '1'

export default defineConfig({
    reporter: 'list',
    retries: 0,
    testDir: path.join(import.meta.dirname, 'tests', 'e2e-hosting'),
    testMatch: '**/*.spec.ts',
    // One worker: the two orgs share a router and their sessions are per-origin
    // browser state, so parallel workers would fight over the same two DBs.
    workers: 1,
    use: {
        // acme is "home"; specs navigate to globex explicitly by absolute URL.
        baseURL: `http://acme.localhost:${PORT}`,
        trace: 'retain-on-failure',
        screenshot: 'only-on-failure',
    },
    projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
    webServer: EXTERNAL
        ? undefined
        : {
              command: 'npx tsx scripts/e2e-hosting.ts',
              // Gate on an org subdomain rather than the bare port: the router
              // answers the apex with its org-finder page well before the
              // tenants are up, so a port-only check would green-light a stack
              // that cannot serve the app.
              url: `http://acme.localhost:${PORT}/api/health`,
              reuseExistingServer: !process.env.CI,
              // Two DB seeds plus a `go run` compile on a cold cache.
              timeout: 300_000,
              stdout: 'pipe',
              stderr: 'pipe',
          },
})
