import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { resolveTestFiles } from '../src/test-files'

// <ws>/{app,doodads}, each with tests/a.test.ts.
function makeWs(): string {
    const ws = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'tcld-unit-')))
    for (const dir of ['app', 'doodads']) {
        fs.mkdirSync(path.join(ws, dir, 'tests'), { recursive: true })
        fs.writeFileSync(path.join(ws, dir, 'tests', 'a.test.ts'), '')
    }
    return ws
}

describe('resolveTestFiles', () => {
    let ws: string
    let pkg: { dir: string; name: string; kind: 'feature' }
    beforeEach(() => {
        ws = makeWs()
        pkg = { dir: path.join(ws, 'doodads'), name: '@tinycld/doodads', kind: 'feature' }
    })
    afterEach(() => fs.rmSync(ws, { recursive: true, force: true }))

    it('resolves a file from the current directory, as a path from the package', () => {
        const files = resolveTestFiles(['tests/a.test.ts'], {
            cwd: pkg.dir,
            workspaceRoot: ws,
            pkg,
        })
        expect(files).toEqual([path.join('tests', 'a.test.ts')])
    })
    it('resolves a file from the workspace root', () => {
        const files = resolveTestFiles(['doodads/tests/a.test.ts'], {
            cwd: pkg.dir,
            workspaceRoot: ws,
            pkg,
        })
        expect(files).toEqual([path.join('tests', 'a.test.ts')])
    })
    it('rejects a file that does not exist', () => {
        expect(() =>
            resolveTestFiles(['tests/none.test.ts'], { cwd: pkg.dir, workspaceRoot: ws, pkg })
        ).toThrow(/not found/)
    })
    it('rejects a file of another member', () => {
        expect(() =>
            resolveTestFiles(['app/tests/a.test.ts'], { cwd: pkg.dir, workspaceRoot: ws, pkg })
        ).toThrow(/not inside @tinycld\/doodads/)
    })
    it('returns no files for no arguments', () => {
        expect(resolveTestFiles([], { cwd: pkg.dir, workspaceRoot: ws, pkg })).toEqual([])
    })
})
