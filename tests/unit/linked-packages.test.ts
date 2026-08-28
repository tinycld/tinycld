import * as fs from 'node:fs'
import * as path from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * CI assembles the lean shell — app + core, no `--with <feature>` flags (see
 * .github/workflows/ci.yml). The only packages linked there are the E2E stubs
 * the workflow scaffolds. This asserts that stays true.
 *
 * WHY IT IS CI-ONLY. A developer's workspace is assembled from whichever
 * members they chose, so locally this set is legitimately mail, drive, calendar
 * and the rest. There is no violation to detect on a dev machine — only in the
 * one environment whose linked set is supposed to be fixed. Running it
 * everywhere would fail for every developer with a feature installed.
 *
 * WHAT IT CATCHES. A dependency added to core or to the app shell that drags a
 * feature package into the lean assembly. That breaks the lean-shell guarantee
 * (a feature-less workspace must typecheck, boot and pass its tests), and it is
 * otherwise invisible: whoever added it has that package linked locally and
 * sees green.
 *
 * It reads the GENERATED config rather than importing `packageRegistry`,
 * because tests/unit-setup.ts mocks `@tinycld/app-generated/tinycld-config` to
 * `[]` to break an import cycle — an import-based check reads an empty registry
 * and passes vacuously no matter what is linked.
 */

const APP_DIR = path.resolve(import.meta.dirname, '../..')

/**
 * Scaffolded by the CI workflow for the keyboard-shortcut and search E2E specs
 * (tests/scripts/scaffold-shortcut-stub.ts, scaffold-search-stubs.ts). They are
 * fixtures, not features — gitignored at the workspace root, generated per run
 * — so they are expected in the lean assembly.
 */
const STUB_SLUGS = ['shortcut-stub', 'search-alpha', 'search-beta']

/** Core is the app's own dependency; app-generated is the generator's output. */
const NON_PACKAGE_NAMES = ['core', 'app-generated']

function linkedPackageSlugs(): string[] {
    const configPath = path.join(APP_DIR, 'tinycld.config.ts')
    const source = fs.readFileSync(configPath, 'utf8')
    const names = [...source.matchAll(/@tinycld\/([a-z0-9-]+)/g)].map(m => m[1])
    return [...new Set(names)].filter(name => !NON_PACKAGE_NAMES.includes(name))
}

describe.runIf(process.env.CI)('linked packages (CI lean shell)', () => {
    it('links nothing beyond core and the E2E stubs', () => {
        const unexpected = linkedPackageSlugs().filter(slug => !STUB_SLUGS.includes(slug))
        // A feature package here means something now depends on it in an
        // assembly that is meant to have none. Remove the dependency — do not
        // add the slug to STUB_SLUGS.
        expect(unexpected).toEqual([])
    })

    it('reads a config that actually lists the stubs (guards the parse)', () => {
        // Without this, a moved file or changed config shape yields an empty
        // list and the check above passes vacuously.
        expect(linkedPackageSlugs().sort()).toEqual([...STUB_SLUGS].sort())
    })
})
