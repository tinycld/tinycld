import { execFileSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import {
    SMOKE_STUB_SLUG,
    scaffoldSmokeStub,
    unregisterSmokeStub,
} from '../../tests/scripts/scaffold-smoke-stub'

const APP_DIR = path.resolve(import.meta.dirname, '..', '..')
const WS_ROOT = path.resolve(APP_DIR, '..')
const ORG_DIR = path.join(APP_DIR, 'app', 'a', '[orgSlug]')

// Uniquely-named app-owned sentinels under [orgSlug] — must survive generation
// (the generator must clean only per-package slug dirs, not app-owned files).
// Kept in THIS file (the only one that invokes the real generator) so two test
// files never run the generator against the shared workspace concurrently.
const rootSentinel = path.join(ORG_DIR, '_preserve_test_root.tsx')
const subDir = path.join(ORG_DIR, '_preserve_test_dir')
const subSentinel = path.join(subDir, 'file.tsx')

// The smoke test asserts the generator emits a real feature package's artifacts.
// Rather than hard-code a first-party slug (contacts/mail/…) that only exists in
// a full local checkout, we scaffold a feature via BOOTSTRAP (--preset full) so
// the fixture matches whatever shape bootstrap produces and is present in every
// assembly (CI app+core, or full local). We assert on that stub's slug.
let stubDir: string
// True only when THIS run created the stub (a clean CI checkout). A developer's
// full local workspace may already have smoke-stub — then we leave the dir in
// place on cleanup and only unregister, to avoid clobbering their state.
let createdStub = false

describe('generate.ts (smoke, real workspace)', () => {
    beforeAll(() => {
        createdStub = !fs.existsSync(path.join(WS_ROOT, SMOKE_STUB_SLUG))
        stubDir = scaffoldSmokeStub()
    }, 120_000)

    afterAll(() => {
        unregisterSmokeStub()
        // Remove the generated route re-exports for the stub.
        const stubRoutes = path.join(ORG_DIR, SMOKE_STUB_SLUG)
        if (fs.existsSync(stubRoutes)) fs.rmSync(stubRoutes, { recursive: true, force: true })
        // Only delete the stub source dir if we created it this run.
        if (createdStub && stubDir && fs.existsSync(stubDir)) {
            fs.rmSync(stubDir, { recursive: true, force: true })
        }
        // The test's generator run left tinycld.config.ts / tinycld.seeds.ts
        // referencing smoke-stub. Now that it's gone, regenerate so the
        // (gitignored) build artifacts match the real present-member set —
        // otherwise a subsequent typecheck in the same job fails on the stale
        // @tinycld/smoke-stub imports.
        try {
            execFileSync('node_modules/.bin/tsx', ['tinycld/scripts/generate.ts'], {
                cwd: WS_ROOT,
                stdio: 'pipe',
            })
        } catch {
            // best-effort: a failed regen here shouldn't mask the test result
        }
    })

    it('produces config/routes/help/uniwind for a bootstrap-scaffolded feature AND preserves app-owned [orgSlug] files', () => {
        // Seed app-owned sentinels before generating.
        fs.mkdirSync(subDir, { recursive: true })
        fs.writeFileSync(rootSentinel, '// app-owned root file, must survive generation\n')
        fs.writeFileSync(subSentinel, '// app-owned subdir file, must survive generation\n')
        try {
            execFileSync('node_modules/.bin/tsx', ['tinycld/scripts/generate.ts'], {
                cwd: WS_ROOT,
                stdio: 'pipe',
            })
            // App-owned files under [orgSlug] survive a generator run.
            expect(fs.existsSync(rootSentinel)).toBe(true)
            expect(fs.existsSync(subSentinel)).toBe(true)
        } finally {
            if (fs.existsSync(rootSentinel)) fs.rmSync(rootSentinel)
            if (fs.existsSync(subDir)) fs.rmSync(subDir, { recursive: true, force: true })
        }
        const config = fs.readFileSync(path.join(APP_DIR, 'tinycld.config.ts'), 'utf8')
        expect(config).toContain('export const tinycldConfig')
        expect(config).toContain(`@tinycld/${SMOKE_STUB_SLUG}`)
        expect(config).toContain('export type MergedPackageSchema')

        const routeIndex = path.join(ORG_DIR, SMOKE_STUB_SLUG, 'index.tsx')
        expect(fs.existsSync(routeIndex)).toBe(true)
        expect(fs.readFileSync(routeIndex, 'utf8')).toContain(
            `from '@tinycld/${SMOKE_STUB_SLUG}/screens/index'`
        )

        expect(fs.existsSync(path.join(APP_DIR, 'lib', 'generated', 'package-help.ts'))).toBe(true)
        const css = fs.readFileSync(
            path.join(APP_DIR, 'lib', 'generated', 'uniwind-sources.css'),
            'utf8'
        )
        expect(css).toContain('@source')
        expect(css).toContain(SMOKE_STUB_SLUG)

        // tinycld.seeds.ts
        expect(fs.existsSync(path.join(APP_DIR, 'tinycld.seeds.ts'))).toBe(true)
        expect(fs.readFileSync(path.join(APP_DIR, 'tinycld.seeds.ts'), 'utf8')).toContain(
            'tinycldSeeds'
        )

        // the @tinycld/app-generated/tinycld-config re-export shim (core imports through it).
        // Named re-exports (not `export *`) are required: a wildcard leaves
        // tinycldConfig undefined under vitest's ESM transform while the
        // pocketbase import cycle resolves. See generate.ts for the rationale.
        const shim = path.join(APP_DIR, 'lib', 'generated', 'tinycld-config.ts')
        expect(fs.existsSync(shim)).toBe(true)
        const shimSrc = fs.readFileSync(shim, 'utf8')
        expect(shimSrc).toContain("export { tinycldConfig } from '../../tinycld.config'")
        expect(shimSrc).toContain("export type { MergedPackageSchema } from '../../tinycld.config'")
    })
})
