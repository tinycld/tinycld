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
        pkgDir = path.join(tmp, 'doodads')
        fs.mkdirSync(path.join(pkgDir, 'tinycld', 'doodads', 'screens'), { recursive: true })
        fs.writeFileSync(path.join(pkgDir, 'tinycld', 'doodads', 'screens', 'index.tsx'), '')
        fs.writeFileSync(path.join(pkgDir, 'tinycld', 'doodads', 'screens', '[id].tsx'), '')
        routesBase = path.join(tmp, 'app', 'a', '(app)')
    })
    afterEach(() => fs.rmSync(tmp, { recursive: true, force: true }))

    it('emits one re-export per screen file under routesBase/<slug>/', () => {
        const written = emitRoutes({
            packageName: '@tinycld/doodads',
            slug: 'doodads',
            packageDir: pkgDir,
            routesDir: 'tinycld/doodads/screens', // resolved path relative to pkgDir
            importSubpath: 'screens', // the exports-map subpath
            routesBase,
        })
        const indexFile = path.join(routesBase, 'doodads', 'index.tsx')
        expect(fs.existsSync(indexFile)).toBe(true)
        expect(fs.readFileSync(indexFile, 'utf8')).toBe(
            "export { default } from '@tinycld/doodads/screens/index'\n"
        )
        const idFile = path.join(routesBase, 'doodads', '[id].tsx')
        expect(fs.readFileSync(idFile, 'utf8')).toBe(
            "export { default } from '@tinycld/doodads/screens/[id]'\n"
        )
        expect(written).toHaveLength(2)
    })

    it('rejects a packageName that would break out of the generated re-export', () => {
        expect(() =>
            emitRoutes({
                packageName: "@tinycld/x'; evil()//",
                slug: 'doodads',
                packageDir: pkgDir,
                routesDir: 'tinycld/doodads/screens',
                importSubpath: 'screens',
                routesBase,
            })
        ).toThrow(/unsafe value/)
    })
})

describe('emitPublicRoutes', () => {
    let tmp: string
    let pkgDir: string
    let publicRoutesBase: string
    beforeEach(() => {
        tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'tcld-public-routes-'))
        pkgDir = path.join(tmp, 'cogs')
        fs.mkdirSync(path.join(pkgDir, 'tinycld', 'cogs', 'public-screens', 'share'), {
            recursive: true,
        })
        fs.writeFileSync(
            path.join(pkgDir, 'tinycld', 'cogs', 'public-screens', 'share', '[token].tsx'),
            ''
        )
        publicRoutesBase = path.join(tmp, 'app', 'p')
    })
    afterEach(() => fs.rmSync(tmp, { recursive: true, force: true }))

    it('emits re-exports under publicRoutesBase/<slug>/, preserving nested paths', () => {
        const written = emitPublicRoutes({
            packageName: '@tinycld/cogs',
            slug: 'cogs',
            packageDir: pkgDir,
            routesDir: 'tinycld/cogs/public-screens',
            importSubpath: 'public-screens',
            publicRoutesBase,
        })
        const tokenFile = path.join(publicRoutesBase, 'cogs', 'share', '[token].tsx')
        expect(fs.existsSync(tokenFile)).toBe(true)
        expect(fs.readFileSync(tokenFile, 'utf8')).toBe(
            "export { default } from '@tinycld/cogs/public-screens/share/[token]'\n"
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
        fs.mkdirSync(path.join(base, 'gizmos'))
        // an app-owned dir
        fs.mkdirSync(path.join(base, 'admin'))
        // app-owned files (must never be touched — prune is dir-only)
        fs.writeFileSync(path.join(base, '_layout.tsx'), '')
        fs.writeFileSync(path.join(base, 'index.tsx'), '')
    })
    afterEach(() => fs.rmSync(base, { recursive: true, force: true }))

    it('removes only orphan package route dirs, sparing present + app-owned + files', () => {
        const pruned = pruneOrphanRouteDirs(base, new Set(['gizmos']), new Set(['admin']))

        expect(pruned).toEqual(['todo'])
        expect(fs.existsSync(path.join(base, 'todo'))).toBe(false)
        expect(fs.existsSync(path.join(base, 'gizmos'))).toBe(true)
        expect(fs.existsSync(path.join(base, 'admin'))).toBe(true)
        expect(fs.existsSync(path.join(base, '_layout.tsx'))).toBe(true)
        expect(fs.existsSync(path.join(base, 'index.tsx'))).toBe(true)
    })

    it('is a no-op when the base dir does not exist', () => {
        expect(pruneOrphanRouteDirs(path.join(base, 'nope'), new Set(), new Set())).toEqual([])
    })

    it('prunes nothing when every dir is present or app-owned', () => {
        expect(pruneOrphanRouteDirs(base, new Set(['gizmos', 'todo']), new Set(['admin']))).toEqual(
            []
        )
        expect(fs.existsSync(path.join(base, 'todo'))).toBe(true)
    })
})
