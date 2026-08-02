import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

const appRoot = join(__dirname, '..', '..')
const entrySource = readFileSync(join(appRoot, 'index.js'), 'utf8')
const packageJson = JSON.parse(readFileSync(join(appRoot, 'package.json'), 'utf8'))
const layoutSource = readFileSync(join(appRoot, 'app', '_layout.tsx'), 'utf8')

// The bug these guard: expo-router's entry pulls in `_ctx`, a require.context over
// app/, which eagerly evaluates the ENTIRE route tree before app/_layout.tsx's own
// body runs. A polyfill imported from app/_layout.tsx therefore installs thousands
// of modules too late — measured on the emitted iOS bundle, core/lib/pocketbase.ts
// (which calls crypto.randomUUID() at module scope via @tanstack/db) initialized at
// position 1682 while lib/polyfill-crypto.ts ran at 4669, so the app died on Hermes
// with "Property 'crypto' doesn't exist" before any handler could report it.
// The only position that wins that race is ahead of the expo-router import.
const POLYFILLS = ['./lib/diagnose-regexp', './lib/polyfill-dom-shim', './lib/polyfill-crypto']

describe('bundle entry', () => {
    it('is the package main, so the polyfills are not bypassed', () => {
        expect(packageJson.main).toBe('index.js')
    })

    it('imports every polyfill before expo-router/entry', () => {
        const routerIndex = entrySource.indexOf("'expo-router/entry'")
        expect(routerIndex).toBeGreaterThan(-1)

        for (const polyfill of POLYFILLS) {
            const polyfillIndex = entrySource.indexOf(`'${polyfill}'`)
            expect(polyfillIndex, `${polyfill} must be imported in index.js`).toBeGreaterThan(-1)
            expect(
                polyfillIndex,
                `${polyfill} must be imported before expo-router/entry`
            ).toBeLessThan(routerIndex)
        }
    })

    it('uses relative specifiers, which resolve without tsconfig paths', () => {
        // index.js is the bundle entry, so it resolves before anything that could
        // set up the `~/*` alias. A `~/lib/...` import here is a latent break.
        expect(entrySource).not.toMatch(/from\s+['"]~\//)
        expect(entrySource).not.toMatch(/import\s+['"]~\//)
    })

    it('does not re-import the polyfills from app/_layout.tsx', () => {
        // Not just redundant: keeping them here invites someone to "fix" ordering
        // by editing _layout.tsx, which cannot work (see above).
        for (const polyfill of POLYFILLS) {
            const aliased = polyfill.replace('./', '~/')
            expect(layoutSource, `${aliased} belongs in index.js`).not.toContain(`'${aliased}'`)
        }
    })
})
