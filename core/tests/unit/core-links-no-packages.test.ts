import * as fs from 'node:fs'
import * as path from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * Core must never LINK a feature package.
 *
 * The rule is about linkage, not names. Core may NAME a package to ask whether
 * it is installed — `usePackage('mail')` returns null in an assembly without
 * mail, and core degrades. What core may not do is take a dependency that has
 * to resolve at build time.
 *
 * WHY. A workspace is assembled per-developer from whichever members that
 * developer chose, so any package may be absent. A linked package makes the
 * lean-shell guarantee (a feature-less workspace must typecheck, boot and pass
 * its tests) stop holding, and the breakage stays invisible to anyone whose
 * assembly happens to include that package.
 *
 * WHAT THIS CHECKS. Core's own manifest, and what pnpm linked from it. Not the
 * source: a predecessor scanned for quoted slugs, could not tell a presence
 * check from an import, and reported 156 violations that were overwhelmingly
 * doc comments and test fixtures. Do not reintroduce a name scan.
 *
 * NOT the app's `tinycld.config.ts` either — that is the APP SHELL's linked set
 * (every feature the developer assembled, correctly so). It says nothing about
 * core. Conflating the two is what produced the 156.
 *
 * Nor can this import `packageRegistry`: tests/unit-setup.ts mocks
 * `@tinycld/app-generated/tinycld-config` to `[]` to break an import cycle, so
 * an import-based check reads an empty registry and passes vacuously.
 */

const CORE_DIR = path.resolve(import.meta.dirname, '../..')

/**
 * Packages core may depend on. `core` is itself (the self-alias that lets it
 * typecheck standalone), `app-generated` is emitted by the generator, and
 * `package-scripts` is the tooling CLI. None is a feature.
 */
const ALLOWED = new Set(['core', 'app-generated', 'package-scripts'])

describe('core links no feature package', () => {
    it('declares no @tinycld feature package in its manifest', () => {
        const pkg = JSON.parse(fs.readFileSync(path.join(CORE_DIR, 'package.json'), 'utf8'))
        const fields = [
            'dependencies',
            'devDependencies',
            'peerDependencies',
            'optionalDependencies',
        ]
        const declared = fields.flatMap(field =>
            Object.keys(pkg[field] ?? {})
                .filter(name => name.startsWith('@tinycld/'))
                .map(name => name.slice('@tinycld/'.length))
                .filter(slug => !ALLOWED.has(slug))
                .map(slug => `${field}: @tinycld/${slug}`)
        )
        expect(declared).toEqual([])
    })

    it('has no @tinycld feature package linked into node_modules', () => {
        // What pnpm actually resolved from the manifest above. Catches a link
        // that arrived without a manifest entry — a stray `pnpm add` inside the
        // member, or an install run from the wrong directory.
        const dir = path.join(CORE_DIR, 'node_modules', '@tinycld')
        if (!fs.existsSync(dir)) return
        const linked = fs.readdirSync(dir).filter(name => !ALLOWED.has(name))
        expect(linked).toEqual([])
    })

    it('requires no member Go module', () => {
        // The server half of the same rule: core's go.mod must require no
        // tinycld.org/<member> module. Its own `module tinycld.org/core` line
        // is not a require and is excluded by the leading-tab match.
        const goMod = fs.readFileSync(path.join(CORE_DIR, 'server', 'go.mod'), 'utf8')
        const required = [...goMod.matchAll(/^\t(tinycld\.org\/[a-z0-9-/]+)/gm)]
            .map(m => m[1])
            .filter(mod => mod !== 'tinycld.org/core')
        expect(required).toEqual([])
    })
})
