import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { emitPublicRoutes, emitRoutes, pruneOrphanRouteDirs } from '../gen-routes'

describe('emitRoutes', () => {
    let tmp: string
    let pkgDir: string
    let routesBase: string
    beforeEach(() => {
        tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'tcld-routes-'))
        pkgDir = path.join(tmp, 'contacts')
        fs.mkdirSync(path.join(pkgDir, 'tinycld', 'contacts', 'screens'), { recursive: true })
        fs.writeFileSync(path.join(pkgDir, 'tinycld', 'contacts', 'screens', 'index.tsx'), '')
        fs.writeFileSync(path.join(pkgDir, 'tinycld', 'contacts', 'screens', '[id].tsx'), '')
        routesBase = path.join(tmp, 'app', 'a', '[orgSlug]')
    })
    afterEach(() => fs.rmSync(tmp, { recursive: true, force: true }))

    it('emits one re-export per screen file under routesBase/<slug>/', () => {
        const written = emitRoutes({
            packageName: '@tinycld/contacts',
            slug: 'contacts',
            packageDir: pkgDir,
            routesDir: 'tinycld/contacts/screens', // resolved path relative to pkgDir
            importSubpath: 'screens', // the exports-map subpath
            routesBase,
        })
        const indexFile = path.join(routesBase, 'contacts', 'index.tsx')
        expect(fs.existsSync(indexFile)).toBe(true)
        expect(fs.readFileSync(indexFile, 'utf8')).toBe(
            "export { default } from '@tinycld/contacts/screens/index'\n"
        )
        const idFile = path.join(routesBase, 'contacts', '[id].tsx')
        expect(fs.readFileSync(idFile, 'utf8')).toBe(
            "export { default } from '@tinycld/contacts/screens/[id]'\n"
        )
        expect(written).toHaveLength(2)
    })
})

describe('emitPublicRoutes', () => {
    let tmp: string
    let pkgDir: string
    let publicRoutesBase: string
    beforeEach(() => {
        tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'tcld-public-routes-'))
        pkgDir = path.join(tmp, 'drive')
        fs.mkdirSync(path.join(pkgDir, 'tinycld', 'drive', 'public-screens', 'share'), {
            recursive: true,
        })
        fs.writeFileSync(
            path.join(pkgDir, 'tinycld', 'drive', 'public-screens', 'share', '[token].tsx'),
            ''
        )
        publicRoutesBase = path.join(tmp, 'app', 'p')
    })
    afterEach(() => fs.rmSync(tmp, { recursive: true, force: true }))

    it('emits re-exports under publicRoutesBase/<slug>/, preserving nested paths', () => {
        const written = emitPublicRoutes({
            packageName: '@tinycld/drive',
            slug: 'drive',
            packageDir: pkgDir,
            routesDir: 'tinycld/drive/public-screens',
            importSubpath: 'public-screens',
            publicRoutesBase,
        })
        const tokenFile = path.join(publicRoutesBase, 'drive', 'share', '[token].tsx')
        expect(fs.existsSync(tokenFile)).toBe(true)
        expect(fs.readFileSync(tokenFile, 'utf8')).toBe(
            "export { default } from '@tinycld/drive/public-screens/share/[token]'\n"
        )
        expect(written).toEqual([tokenFile])
    })
})

describe('pruneOrphanRouteDirs', () => {
    let base: string
    beforeEach(() => {
        base = fs.mkdtempSync(path.join(os.tmpdir(), 'tcld-prune-'))
        // an orphan package route dir (package since removed)
        fs.mkdirSync(path.join(base, 'todo'))
        fs.writeFileSync(path.join(base, 'todo', '[id].tsx'), '')
        // a present package's route dir
        fs.mkdirSync(path.join(base, 'mail'))
        // an app-owned dir
        fs.mkdirSync(path.join(base, 'admin'))
        // app-owned files (must never be touched — prune is dir-only)
        fs.writeFileSync(path.join(base, '_layout.tsx'), '')
        fs.writeFileSync(path.join(base, 'index.tsx'), '')
    })
    afterEach(() => fs.rmSync(base, { recursive: true, force: true }))

    it('removes only orphan package route dirs, sparing present + app-owned + files', () => {
        const pruned = pruneOrphanRouteDirs(base, new Set(['mail']), new Set(['admin']))

        expect(pruned).toEqual(['todo'])
        expect(fs.existsSync(path.join(base, 'todo'))).toBe(false)
        expect(fs.existsSync(path.join(base, 'mail'))).toBe(true)
        expect(fs.existsSync(path.join(base, 'admin'))).toBe(true)
        expect(fs.existsSync(path.join(base, '_layout.tsx'))).toBe(true)
        expect(fs.existsSync(path.join(base, 'index.tsx'))).toBe(true)
    })

    it('is a no-op when the base dir does not exist', () => {
        expect(pruneOrphanRouteDirs(path.join(base, 'nope'), new Set(), new Set())).toEqual([])
    })

    it('prunes nothing when every dir is present or app-owned', () => {
        expect(pruneOrphanRouteDirs(base, new Set(['mail', 'todo']), new Set(['admin']))).toEqual(
            []
        )
        expect(fs.existsSync(path.join(base, 'todo'))).toBe(true)
    })
})
