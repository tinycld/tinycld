import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { readCoreVersions, renderOverridesBlock, writeWorkspaceRoot } from '../write-workspace-root'

const PINS = {
    '//': 'doc',
    expo: '55.0.26',
    '@tanstack/db': '0.8.5',
    'react-native-drax': 'github:nathanstitt/react-native-drax#b863d89',
}

function makeFixtureRoot(): { wsRoot: string; appDir: string } {
    const wsRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'wsroot-'))
    const appDir = path.join(wsRoot, 'tinycld')
    fs.mkdirSync(path.join(appDir, 'core'), { recursive: true })
    fs.writeFileSync(path.join(appDir, 'core', 'package-versions.json'), JSON.stringify(PINS))
    fs.writeFileSync(
        path.join(appDir, 'biome.json'),
        JSON.stringify({ root: false, files: { includes: ['**/*.ts', '!lib/generated'] } })
    )
    return { wsRoot, appDir }
}

afterEach(() => {
    vi.unstubAllEnvs()
})

describe('renderOverridesBlock', () => {
    it('matches the Go renderer byte-for-byte: sorted, scoped names quoted, plain bare', () => {
        const block = renderOverridesBlock({ expo: '55.0.26', '@tanstack/db': '0.8.5', uniwind: '1.8.0' })
        expect(block).toBe(['overrides:', "  '@tanstack/db': 0.8.5", '  expo: 55.0.26', '  uniwind: 1.8.0'].join('\n'))
    })
})

describe('readCoreVersions', () => {
    it('strips the // doc key', () => {
        const { appDir } = makeFixtureRoot()
        const pins = readCoreVersions(appDir)
        expect(pins['//']).toBeUndefined()
        expect(pins.expo).toBe('55.0.26')
    })
    it('hard-errors when the table is missing', () => {
        const { appDir } = makeFixtureRoot()
        fs.rmSync(path.join(appDir, 'core', 'package-versions.json'))
        expect(() => readCoreVersions(appDir)).toThrow(/cannot read/)
    })
    it('hard-errors on malformed JSON', () => {
        const { appDir } = makeFixtureRoot()
        fs.writeFileSync(path.join(appDir, 'core', 'package-versions.json'), '{nope')
        expect(() => readCoreVersions(appDir)).toThrow(/malformed/)
    })
    it('hard-errors when the table has no pins beyond the doc key', () => {
        const { appDir } = makeFixtureRoot()
        fs.writeFileSync(path.join(appDir, 'core', 'package-versions.json'), JSON.stringify({ '//': 'doc' }))
        expect(() => readCoreVersions(appDir)).toThrow(/no version pins/)
    })
})

describe('writeWorkspaceRoot', () => {
    it('writes pnpm-workspace.yaml with the overrides block derived from the core table', () => {
        const { wsRoot, appDir } = makeFixtureRoot()
        writeWorkspaceRoot(wsRoot, appDir)
        const yaml = fs.readFileSync(path.join(wsRoot, 'pnpm-workspace.yaml'), 'utf8')
        expect(yaml).toContain('nodeLinker: hoisted')
        expect(yaml).toContain("  '@tanstack/db': 0.8.5")
        expect(yaml).toContain('  expo: 55.0.26')
        expect(yaml).not.toContain('storeDir:')
    })
    it('writes the root package-versions.json as a derived copy of the core table', () => {
        const { wsRoot, appDir } = makeFixtureRoot()
        writeWorkspaceRoot(wsRoot, appDir)
        const derived = JSON.parse(fs.readFileSync(path.join(wsRoot, 'package-versions.json'), 'utf8'))
        expect(derived.expo).toBe('55.0.26')
        expect(derived['//']).toMatch(/derived/i)
    })
    it('preserves human-owned fields in an existing root package.json', () => {
        const { wsRoot, appDir } = makeFixtureRoot()
        fs.writeFileSync(
            path.join(wsRoot, 'package.json'),
            JSON.stringify({ name: '@tinycld/workspace', scripts: { 'docker:ssl': 'x' }, license: 'AGPL-3.0-only' })
        )
        writeWorkspaceRoot(wsRoot, appDir)
        const pkg = JSON.parse(fs.readFileSync(path.join(wsRoot, 'package.json'), 'utf8'))
        expect(pkg.scripts['docker:ssl']).toBe('x')
        expect(pkg.license).toBe('AGPL-3.0-only')
        expect(pkg.scripts.postinstall).toContain('link-members')
        expect(pkg.packageManager).toMatch(/^pnpm@/)
    })
    it('writes the root biome.json inlined from the canonical with rerooted globs', () => {
        const { wsRoot, appDir } = makeFixtureRoot()
        writeWorkspaceRoot(wsRoot, appDir)
        const biome = JSON.parse(fs.readFileSync(path.join(wsRoot, 'biome.json'), 'utf8'))
        expect(biome.root).toBe(true)
        expect(biome.files.includes).toContain('!tinycld/lib/generated')
        expect(biome.vcs.root).toBe('tinycld')
    })
    it('warns to reinstall when pins changed from the previous derived copy', () => {
        const { wsRoot, appDir } = makeFixtureRoot()
        writeWorkspaceRoot(wsRoot, appDir)
        const table = JSON.parse(fs.readFileSync(path.join(appDir, 'core', 'package-versions.json'), 'utf8'))
        table.expo = '56.0.0'
        fs.writeFileSync(path.join(appDir, 'core', 'package-versions.json'), JSON.stringify(table))
        const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
        writeWorkspaceRoot(wsRoot, appDir)
        expect(warn.mock.calls.flat().join(' ')).toMatch(/run pnpm install again/)
        warn.mockRestore()
    })
    it('under TINYCLD_SERVER_REBUILD writes only biome.json and leaves pin files frozen', () => {
        const { wsRoot, appDir } = makeFixtureRoot()
        fs.writeFileSync(path.join(wsRoot, 'pnpm-workspace.yaml'), 'storeDir: /baked\n')
        fs.writeFileSync(path.join(wsRoot, 'package-versions.json'), '{"expo":"55.0.0"}')
        vi.stubEnv('TINYCLD_SERVER_REBUILD', '1')
        writeWorkspaceRoot(wsRoot, appDir)
        expect(fs.readFileSync(path.join(wsRoot, 'pnpm-workspace.yaml'), 'utf8')).toBe('storeDir: /baked\n')
        expect(fs.readFileSync(path.join(wsRoot, 'package-versions.json'), 'utf8')).toBe('{"expo":"55.0.0"}')
        expect(fs.existsSync(path.join(wsRoot, 'biome.json'))).toBe(true)
        expect(fs.existsSync(path.join(wsRoot, 'package.json'))).toBe(false)
    })
    it('is idempotent — a second run produces byte-identical files', () => {
        const { wsRoot, appDir } = makeFixtureRoot()
        writeWorkspaceRoot(wsRoot, appDir)
        const first = fs.readFileSync(path.join(wsRoot, 'pnpm-workspace.yaml'), 'utf8')
        writeWorkspaceRoot(wsRoot, appDir)
        expect(fs.readFileSync(path.join(wsRoot, 'pnpm-workspace.yaml'), 'utf8')).toBe(first)
    })
})
