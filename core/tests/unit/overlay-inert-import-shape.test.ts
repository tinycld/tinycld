// @vitest-environment node

import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const OVERLAY_DIR = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'ui', 'overlay')

function source(file: string): string {
    return readFileSync(join(OVERLAY_DIR, file), 'utf8')
}

function importSpecifiers(code: string): string[] {
    return [...code.matchAll(/^import[^'"]*['"]([^'"]+)['"]/gm)].map(m => m[1])
}

/**
 * The inert machinery must stay a leaf, reachable without pulling in the
 * overlay engine.
 *
 * `d5e2875c` shipped a web bundle in which `inertExemptionEpoch` was undefined
 * at runtime — a `ReferenceError` behind the error boundary — while tsc and
 * vitest were both green. A bundler orders a cycle differently from the test
 * runner, so a binding can be read before its module body has run, and a
 * barrel that re-exports the engine is the easiest way to create one: a module
 * that only wants to stay interactive ends up importing `host.tsx`, which
 * imports back.
 *
 * These assertions are cheap and run in every `tinycld-pkg check`, which is
 * where this class of break needs to surface — not in an e2e that has to
 * export a bundle first.
 */
describe('the inert machinery is a leaf', () => {
    it('inert-siblings imports nothing at all', () => {
        expect(importSpecifiers(source('inert-siblings.ts'))).toEqual([])
    })

    // It may reach the leaf and React, and nothing else in the engine.
    it('use-inert-exempt imports only react, react-native and the leaf', () => {
        const specifiers = importSpecifiers(source('use-inert-exempt.ts'))
        expect(specifiers.sort()).toEqual(['./inert-siblings', 'react', 'react-native'])
    })

    // The barrel pulls in host.tsx, so re-exporting the inert machinery from
    // it would put the whole engine in the graph of every consumer.
    it('the barrel does not re-export the inert machinery', () => {
        // The specifiers only — the file's doc comment names both modules on
        // purpose, to say why they are absent.
        const specifiers = importSpecifiers(source('index.ts'))
        const reexports = [...source('index.ts').matchAll(/from\s+['"]([^'"]+)['"]/g)].map(
            m => m[1]
        )
        for (const spec of [...specifiers, ...reexports]) {
            expect(spec).not.toContain('inert-siblings')
            expect(spec).not.toContain('use-inert-exempt')
        }
    })

    // The shape that actually ships: a consumer reaches the hook directly.
    it('its consumers import it directly, not through the barrel', async () => {
        const mod = await import('@tinycld/core/ui/overlay/use-inert-exempt')
        expect(typeof mod.useInertExempt).toBe('function')

        const leaf = await import('@tinycld/core/ui/overlay/inert-siblings')
        expect(typeof leaf.inertExemptionEpoch).toBe('function')
        expect(typeof leaf.applyInertSiblings).toBe('function')
        // The binding that was undefined in the broken bundle. Reading it
        // here is the point: it must resolve without the engine loaded.
        expect(leaf.inertExemptionEpoch()).toBeTypeOf('number')
    })
})
