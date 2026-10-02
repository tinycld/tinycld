import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { describe, expect, it } from 'vitest'
import {
    buildBundledPackages,
    buildGoWork,
    buildMemberGoWork,
    buildPackageExtensionsGo,
    replaceSymlink,
    type ServerPkg,
} from '../gen-server'

const doodads: ServerPkg = {
    slug: 'doodads',
    module: 'tinycld.org/packages/doodads',
    serverRelPath: '../../doodads/server',
}

describe('buildPackageExtensionsGo', () => {
    it('emits a no-op when no packages have servers', () => {
        const go = buildPackageExtensionsGo([])
        expect(go).toContain('func registerPackageExtensions(_ *pocketbase.PocketBase) {}')
    })
    it('imports + registers each server package by slug identifier', () => {
        const go = buildPackageExtensionsGo([doodads])
        expect(go).toContain('doodads "tinycld.org/packages/doodads"')
        expect(go).toContain('doodads.Register(app)')
        expect(go).toContain('func registerPackageExtensions(app *pocketbase.PocketBase)')
    })

    it('rejects a module path that would break out of the generated Go import', () => {
        const bad: ServerPkg = { ...doodads, module: 'tinycld.org/x"; evil()//' }
        expect(() => buildPackageExtensionsGo([bad])).toThrow(/unsafe value/)
    })
})

describe('buildGoWork', () => {
    it('includes ., core, and each server package use', () => {
        const work = buildGoWork('../../core/server', [doodads])
        expect(work).toContain('use (')
        expect(work).toContain('    .')
        expect(work).toContain('    ../../core/server')
        expect(work).toContain('    ../../doodads/server')
    })

    // core requires its nested archive-format module at v0.0.0; without it in
    // the workspace the graph load hits the proxy for that version.
    it("uses core's nested archive-format module", () => {
        const work = buildGoWork('../../core/server', [doodads])
        expect(work).toContain('    ../../core/server/backup/format')
    })
})

const gopbsReplaceLine =
    'replace github.com/osshield/gopbs => github.com/nathanstitt/gopbs v0.0.0-20260929212904-191e2150ab06'
const webdavReplaceLine =
    'replace github.com/emersion/go-webdav => github.com/nathanstitt/go-webdav v0.7.1-0.20261001184608-67abd707e045'
const forkedReplaceLines = [gopbsReplaceLine, webdavReplaceLine]

describe('buildMemberGoWork', () => {
    it('replaces core so a standalone member build resolves it', () => {
        const work = buildMemberGoWork(
            '../../tinycld/core/server',
            '../../tinycld/third_party/pocketbase',
            forkedReplaceLines
        )
        expect(work).toContain('use .')
        expect(work).toContain('replace tinycld.org/core => ../../tinycld/core/server')
    })

    // core requires the nested archive-format module at v0.0.0. Versioned because
    // core (a `use` member) replaces it at all versions in its own go.mod.
    it('replaces the archive-format module, versioned', () => {
        const work = buildMemberGoWork(
            '../../tinycld/core/server',
            '../../tinycld/third_party/pocketbase',
            forkedReplaceLines
        )
        expect(work).toContain(
            'replace tinycld.org/core/backup/format v0.0.0 => ../../tinycld/core/server/backup/format'
        )
    })

    // The fork is vendored in the app shell, so it is never absent — a member that
    // resolved upstream goja instead would hit a goja<->sobek mismatch at build time.
    it('always replaces the fork so a member never resolves upstream goja', () => {
        const work = buildMemberGoWork(
            '../../tinycld/core/server',
            '../../tinycld/third_party/pocketbase',
            forkedReplaceLines
        )
        expect(work).toContain(
            'replace github.com/pocketbase/pocketbase => ../../tinycld/third_party/pocketbase'
        )
    })

    // core/server/backup/pbs imports the forked gopbs client; a member whose
    // server imports coreserver needs the same replace or a standalone build
    // resolves the unforked module.
    it('carries the gopbs replace verbatim', () => {
        const work = buildMemberGoWork(
            '../../tinycld/core/server',
            '../../tinycld/third_party/pocketbase',
            forkedReplaceLines
        )
        expect(work).toContain(gopbsReplaceLine)
    })

    // go-webdav is forked for its CalDAV PROPPATCH handling. Unlike gopbs the
    // unforked module still COMPILES, so a member missing this replace builds
    // and tests green against a different server than the app ships — the
    // failure is silent, which is why every forked pin is carried, not just the
    // ones that break the build.
    it('carries every forked replace, not only the first', () => {
        const work = buildMemberGoWork(
            '../../tinycld/core/server',
            '../../tinycld/third_party/pocketbase',
            forkedReplaceLines
        )
        expect(work).toContain(webdavReplaceLine)
    })
})

describe('replaceSymlink', () => {
    it('creates a symlink pointing at the target', () => {
        const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'tcld-symlink-'))
        try {
            const target = path.join(tmp, 'real.js')
            fs.writeFileSync(target, 'export {}')
            const link = path.join(tmp, 'sub', 'link.js')
            replaceSymlink(target, link)
            expect(fs.lstatSync(link).isSymbolicLink()).toBe(true)
            expect(fs.readFileSync(link, 'utf8')).toBe('export {}')
        } finally {
            fs.rmSync(tmp, { recursive: true, force: true })
        }
    })

    it('replaces an existing symlink (idempotent re-link)', () => {
        const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'tcld-symlink-'))
        try {
            const a = path.join(tmp, 'a.js')
            const b = path.join(tmp, 'b.js')
            fs.writeFileSync(a, 'A')
            fs.writeFileSync(b, 'B')
            const link = path.join(tmp, 'link.js')
            replaceSymlink(a, link)
            expect(fs.readFileSync(link, 'utf8')).toBe('A')
            // re-link to a different target — must not throw, must repoint
            replaceSymlink(b, link)
            expect(fs.readFileSync(link, 'utf8')).toBe('B')
        } finally {
            fs.rmSync(tmp, { recursive: true, force: true })
        }
    })

    it('removes a broken symlink (target deleted between runs)', () => {
        const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'tcld-symlink-'))
        try {
            const target = path.join(tmp, 'will-disappear.js')
            const newTarget = path.join(tmp, 'replacement.js')
            fs.writeFileSync(target, 'OLD')
            fs.writeFileSync(newTarget, 'NEW')
            const link = path.join(tmp, 'link.js')
            replaceSymlink(target, link)
            fs.unlinkSync(target) // target disappears — link is now broken
            // should not throw and should repoint to newTarget
            replaceSymlink(newTarget, link)
            expect(fs.readFileSync(link, 'utf8')).toBe('NEW')
        } finally {
            fs.rmSync(tmp, { recursive: true, force: true })
        }
    })
})

describe('buildBundledPackages', () => {
    it('maps manifests to the Go pkg_seed shape', () => {
        const json = buildBundledPackages([
            {
                manifest: {
                    name: 'Gizmos',
                    slug: 'gizmos',
                    version: '0.1.0',
                    description: 'Email',
                    nav: { label: 'Gizmos', icon: 'mail', order: 10 },
                    server: { package: 'server', module: 'tinycld.org/packages/gizmos' },
                },
            },
            {
                manifest: { name: 'Trinkets', slug: 'trinkets', version: '0.2.0', description: '' },
            },
        ])
        const parsed = JSON.parse(json)
        expect(parsed).toEqual([
            {
                name: 'Gizmos',
                slug: 'gizmos',
                version: '0.1.0',
                icon: 'mail',
                description: 'Email',
                hasServer: true,
                navOrder: 10,
                manifestJson: JSON.stringify({
                    name: 'Gizmos',
                    slug: 'gizmos',
                    version: '0.1.0',
                    description: 'Email',
                    nav: { label: 'Gizmos', icon: 'mail', order: 10 },
                    server: { package: 'server', module: 'tinycld.org/packages/gizmos' },
                }),
            },
            {
                name: 'Trinkets',
                slug: 'trinkets',
                version: '0.2.0',
                icon: '',
                description: '',
                hasServer: false,
                navOrder: 0,
                manifestJson: JSON.stringify({
                    name: 'Trinkets',
                    slug: 'trinkets',
                    version: '0.2.0',
                    description: '',
                }),
            },
        ])
    })

    it('core bundled row is named "TinyCld Base", has a server, and a source spec', () => {
        const json = buildBundledPackages([
            {
                manifest: {
                    name: 'TinyCld Base',
                    slug: 'core',
                    version: '0.0.4',
                    description: 'The TinyCld base — app shell, core library, and server.',
                },
                source: 'github:tinycld/tinycld',
                hasServer: true,
            },
        ])
        const rows = JSON.parse(json) as Array<{
            slug: string
            name: string
            hasServer: boolean
            source?: string
        }>
        const core = rows.find(r => r.slug === 'core')
        expect(core).toMatchObject({
            name: 'TinyCld Base',
            hasServer: true,
            source: 'github:tinycld/tinycld',
        })
    })
})
