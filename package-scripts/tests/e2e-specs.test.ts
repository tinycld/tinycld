import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { buildSpecFilesCommand, groupSpecFiles } from '../src/e2e-specs'
import { combineMemberConfigs, type LoadedMember, SPECS_ENV } from '../src/multi-member'

// <ws>/{app,doodads,gizmos}: app and doodads have e2e, gizmos has none.
function makeWs(): string {
    const ws = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'tcld-specs-')))
    fs.writeFileSync(path.join(ws, 'package.json'), JSON.stringify({ workspaces: ['app'] }))
    for (const [dir, name, manifest, e2e] of [
        ['app', 'app', false, true],
        ['doodads', '@tinycld/doodads', true, true],
        ['gizmos', '@tinycld/gizmos', true, false],
    ] as const) {
        fs.mkdirSync(path.join(ws, dir, 'tests', 'e2e'), { recursive: true })
        fs.writeFileSync(path.join(ws, dir, 'package.json'), JSON.stringify({ name }))
        if (manifest) fs.writeFileSync(path.join(ws, dir, 'manifest.ts'), 'export default {}')
        if (e2e) fs.writeFileSync(path.join(ws, dir, 'playwright.config.ts'), '')
        for (const spec of ['a.spec.ts', 'b.spec.ts']) {
            fs.writeFileSync(path.join(ws, dir, 'tests', 'e2e', spec), '')
        }
    }
    return ws
}

describe('groupSpecFiles', () => {
    let ws: string
    let opts: { cwd: string; workspaceRoot: string; appDir: string }
    beforeEach(() => {
        ws = makeWs()
        opts = { cwd: ws, workspaceRoot: ws, appDir: path.join(ws, 'app') }
    })
    afterEach(() => fs.rmSync(ws, { recursive: true, force: true }))

    it('groups files by member, each as its absolute path', () => {
        const groups = groupSpecFiles(
            ['app/tests/e2e/a.spec.ts', 'doodads/tests/e2e/a.spec.ts', 'app/tests/e2e/b.spec.ts'],
            opts
        )
        expect(groups).toEqual([
            {
                dir: path.join(ws, 'app'),
                name: 'app',
                files: [
                    path.join(ws, 'app', 'tests', 'e2e', 'a.spec.ts'),
                    path.join(ws, 'app', 'tests', 'e2e', 'b.spec.ts'),
                ],
            },
            {
                dir: path.join(ws, 'doodads'),
                name: '@tinycld/doodads',
                files: [path.join(ws, 'doodads', 'tests', 'e2e', 'a.spec.ts')],
            },
        ])
    })
    it('resolves a file from the current directory before the workspace root', () => {
        const groups = groupSpecFiles(['tests/e2e/b.spec.ts'], {
            ...opts,
            cwd: path.join(ws, 'doodads'),
        })
        expect(groups.map(g => g.name)).toEqual(['@tinycld/doodads'])
    })
    it('rejects a file that does not exist', () => {
        expect(() => groupSpecFiles(['app/tests/e2e/none.spec.ts'], opts)).toThrow(/not found/)
    })
    it('rejects a member without a Playwright config', () => {
        expect(() => groupSpecFiles(['gizmos/tests/e2e/a.spec.ts'], opts)).toThrow(
            /no playwright.config.ts/
        )
    })
})

describe('buildSpecFilesCommand', () => {
    it('is one Playwright run with the combined config and the members in the env', () => {
        const members = [{ dir: '/ws/doodads', name: '@tinycld/doodads', files: ['/ws/x.spec.ts'] }]
        const cmd = buildSpecFilesCommand(members, '/ws/app', ['--reporter=list'])
        expect(cmd.bin).toBe('playwright')
        expect(cmd.args[0]).toBe('test')
        expect(cmd.args[2]).toMatch(/multi-member\.config\.ts$/)
        expect(cmd.args.slice(3)).toEqual(['--reporter=list'])
        expect(cmd.cwd).toBe('/ws/app')
        expect(JSON.parse(cmd.env?.[SPECS_ENV] ?? '')).toEqual(members)
    })
    it('requires the test stubs, so a named spec file never skips silently', () => {
        const cmd = buildSpecFilesCommand([], '/ws/app')
        expect(cmd.env?.TINYCLD_E2E_REQUIRE_STUBS).toBe('1')
    })
})

describe('combineMemberConfigs', () => {
    const stack = {
        webServer: [{ command: 'serve', url: 'http://localhost:1/health' }],
        globalSetup: '/ws/app/tests/setup.ts',
        reporter: 'dot' as const,
    }
    const app: LoadedMember = {
        dir: '/ws/app',
        name: 'app',
        files: ['/ws/app/tests/e2e/a.spec.ts'],
        realTestDir: '/ws/app/tests/e2e',
        config: { ...stack, testDir: '/ws/app/tests/e2e' },
    }
    const doodads: LoadedMember = {
        dir: '/ws/doodads',
        name: '@tinycld/doodads',
        files: ['/ws/doodads/tests/e2e/a.spec.ts', '/ws/doodads/tests/e2e/sub/b.spec.ts'],
        realTestDir: '/ws/doodads/tests/e2e',
        config: {
            ...stack,
            testDir: '/ws/node_modules/@tinycld/doodads/tests/e2e',
            timeout: 90_000,
            projects: [
                { name: 'first', grep: /first/, workers: 1 },
                { name: 'rest', grepInvert: /first/, dependencies: ['first'] },
            ],
        },
    }

    it('keeps one server stack and makes a project for each member', () => {
        const combined = combineMemberConfigs([app, doodads])
        expect(combined.webServer).toEqual(stack.webServer)
        expect(combined.globalSetup).toBe(stack.globalSetup)
        expect(combined.testDir).toBeUndefined()
        expect(combined.projects?.map(p => p.name)).toEqual([
            'app',
            'doodads:first',
            'doodads:rest',
        ])
    })
    it("carries a member's test dir, timeout and own projects", () => {
        const [, first, rest] = combineMemberConfigs([app, doodads]).projects ?? []
        expect(first.testDir).toBe('/ws/node_modules/@tinycld/doodads/tests/e2e')
        expect(first.timeout).toBe(90_000)
        expect(first.workers).toBe(1)
        expect(rest.dependencies).toEqual(['doodads:first'])
    })
    it('selects only the named files, on the real path and on the symlink path', () => {
        const [appProject, first] = combineMemberConfigs([app, doodads]).projects ?? []
        const matches = (project: typeof first, file: string) =>
            (project.testMatch as RegExp[]).some(pattern => pattern.test(file))
        expect(matches(first, '/ws/doodads/tests/e2e/a.spec.ts')).toBe(true)
        expect(matches(first, '/ws/node_modules/@tinycld/doodads/tests/e2e/sub/b.spec.ts')).toBe(
            true
        )
        expect(matches(first, '/ws/doodads/tests/e2e/c.spec.ts')).toBe(false)
        expect(matches(first, '/ws/doodads/tests/e2e/b.spec.ts')).toBe(false)
        expect(matches(appProject, '/ws/app/tests/e2e/sub/b.spec.ts')).toBe(false)
    })
    it('rejects a file outside the test dir of its member', () => {
        const stray = { ...app, files: ['/ws/app/other/a.spec.ts'] }
        expect(() => combineMemberConfigs([stray])).toThrow(/not under the test dir/)
    })
    it('rejects a member that starts its own servers', () => {
        const own = { ...doodads, config: { ...doodads.config, webServer: [] } }
        expect(() => combineMemberConfigs([app, own])).toThrow(/its own servers/)
    })
})
