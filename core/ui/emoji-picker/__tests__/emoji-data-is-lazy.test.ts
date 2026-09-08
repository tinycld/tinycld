import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

// Metro does not tree-shake. A single static `import ... from './emoji-data'`
// anywhere in the graph puts ~99KB of emoji table into the base bundle for
// every user on every platform — including deployments with no package that
// has a picker — and nothing at runtime would reveal it. This test is the only
// thing standing between that and a silent startup regression.

const CORE = join(dirname(fileURLToPath(import.meta.url)), '../../..')

/** The one module allowed to reach the table, and only via import(). */
const LOADER = 'ui/emoji-picker/use-emoji-data.ts'

function sourceFiles(dir: string, found: string[] = []): string[] {
    for (const entry of readdirSync(dir)) {
        if (entry === 'node_modules' || entry.startsWith('.')) continue
        const full = join(dir, entry)
        if (statSync(full).isDirectory()) sourceFiles(full, found)
        else if (/\.tsx?$/.test(entry)) found.push(full)
    }
    return found
}

/** A value import of the module. A `import type` is erased and costs nothing. */
const STATIC_IMPORT = /(?:^|\n)\s*import\s+(?!type\s)[^;\n]*?['"][^'"]*emoji-data['"]/
/** `export * from './emoji-data'` re-exports it just as statically. */
const STATIC_REEXPORT = /(?:^|\n)\s*export\s+(?!type\s)[^;\n]*?from\s+['"][^'"]*emoji-data['"]/

describe('emoji-data stays out of the base bundle', () => {
    const files = sourceFiles(CORE).filter(file => !file.includes('__tests__'))

    it('is never statically imported, by anything', () => {
        const offenders = files
            .filter(file => {
                const source = readFileSync(file, 'utf8')
                return STATIC_IMPORT.test(source) || STATIC_REEXPORT.test(source)
            })
            .map(file => relative(CORE, file))

        expect(offenders).toEqual([])
    })

    it('is reached only by the loader, and there only through import()', () => {
        const importers = files
            .filter(file => /['"][^'"]*emoji-data['"]/.test(readFileSync(file, 'utf8')))
            .map(file => relative(CORE, file))

        expect(importers.sort()).toEqual([LOADER, 'lib/emoji/search.ts'].sort())
        expect(readFileSync(join(CORE, LOADER), 'utf8')).toMatch(
            /import\(\s*['"]\.\/emoji-data['"]\s*\)/
        )
    })

    it('is big enough that the guard matters', () => {
        // If this ever shrinks below ~50KB the lazy plumbing could be dropped;
        // if it grows past ~150KB someone should be told.
        const bytes = statSync(join(CORE, 'ui/emoji-picker/emoji-data.ts')).size
        expect(bytes).toBeGreaterThan(50_000)
        expect(bytes).toBeLessThan(150_000)
    })
})
