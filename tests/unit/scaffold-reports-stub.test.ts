import { spawnSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { isPnpmMember, refreshReportsStub, STUB_DIR } from '../scripts/scaffold-reports-stub'

vi.mock('node:child_process', async importOriginal => ({
    ...(await importOriginal<typeof import('node:child_process')>()),
    spawnSync: vi.fn(),
}))

const TEMPLATES = path.resolve(import.meta.dirname, '..', 'scripts', 'reports-stub')
const MEMBER_YAML = 'packages:\n  - tinycld\n  - salesstub\n\nonlyBuiltDependencies:\n  - esbuild\n'
const NON_MEMBER_YAML = 'packages:\n  - tinycld\n\nonlyBuiltDependencies:\n  - salesstub\n'

let wsRoot = ''

function writeFile(rel: string, contents: string) {
    const target = path.join(wsRoot, rel)
    fs.mkdirSync(path.dirname(target), { recursive: true })
    fs.writeFileSync(target, contents)
}

function readStub(rel: string) {
    return fs.readFileSync(path.join(wsRoot, STUB_DIR, rel), 'utf8')
}

beforeEach(() => {
    wsRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'reports-stub-'))
    writeFile(
        'tinycld/core/server/go.mod',
        'module tinycld.org/core\n\ngo 1.27.3\n\nrequire github.com/pocketbase/pocketbase v0.40.4\n'
    )
    vi.mocked(spawnSync).mockClear()
})

afterEach(() => {
    fs.rmSync(wsRoot, { recursive: true, force: true })
})

describe('isPnpmMember', () => {
    it('finds the stub in the packages block', () => {
        expect(isPnpmMember(MEMBER_YAML)).toBe(true)
    })

    it('ignores the stub name outside the packages block', () => {
        expect(isPnpmMember(NON_MEMBER_YAML)).toBe(false)
    })

    it('is false for a file with no packages block', () => {
        expect(isPnpmMember('onlyBuiltDependencies:\n  - salesstub\n')).toBe(false)
    })
})

describe('refreshReportsStub', () => {
    it('re-writes a member stub from the templates', () => {
        writeFile('pnpm-workspace.yaml', MEMBER_YAML)
        writeFile(`${STUB_DIR}/manifest.ts`, 'stale manifest')
        writeFile(`${STUB_DIR}/server/register.go`, 'package server // stale')

        expect(refreshReportsStub(wsRoot)).toBe(true)

        const template = fs.readFileSync(path.join(TEMPLATES, 'server', 'register.go.txt'), 'utf8')
        expect(readStub('server/register.go')).toBe(template)
        expect(readStub('manifest.ts')).toContain("slug: 'salesstub'")
        expect(readStub('server/go.mod')).toContain('github.com/pocketbase/pocketbase v0.40.4')
        expect(spawnSync).not.toHaveBeenCalled()
        expect(fs.readFileSync(path.join(wsRoot, 'pnpm-workspace.yaml'), 'utf8')).toBe(MEMBER_YAML)
    })

    it('writes nothing and installs nothing when the stub is not a member', () => {
        writeFile('pnpm-workspace.yaml', NON_MEMBER_YAML)

        expect(refreshReportsStub(wsRoot)).toBe(false)

        expect(fs.existsSync(path.join(wsRoot, STUB_DIR))).toBe(false)
        expect(spawnSync).not.toHaveBeenCalled()
        expect(fs.readFileSync(path.join(wsRoot, 'pnpm-workspace.yaml'), 'utf8')).toBe(
            NON_MEMBER_YAML
        )
    })

    it('writes nothing when the stub is listed but its directory is gone', () => {
        writeFile('pnpm-workspace.yaml', MEMBER_YAML)

        expect(refreshReportsStub(wsRoot)).toBe(false)

        expect(fs.existsSync(path.join(wsRoot, STUB_DIR))).toBe(false)
    })
})
