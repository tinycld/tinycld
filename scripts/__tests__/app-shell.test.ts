import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { EXPORTED_SHELL, exportedShellPath, renameStagedShell, SERVED_SHELL } from '../app-shell'

const tmpDirs: string[] = []

const makeDir = () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'app-shell-'))
    tmpDirs.push(dir)
    return dir
}

afterEach(() => {
    while (tmpDirs.length) {
        fs.rmSync(tmpDirs.pop() as string, { recursive: true, force: true })
    }
})

describe('exportedShellPath', () => {
    it('returns the shell path when the export produced one', () => {
        const dir = makeDir()
        fs.writeFileSync(path.join(dir, EXPORTED_SHELL), '<html></html>')
        expect(exportedShellPath(dir)).toBe(path.join(dir, EXPORTED_SHELL))
    })

    it('throws with the expo export command when the shell is missing', () => {
        expect(() => exportedShellPath(makeDir())).toThrow(/expo export --platform web/)
    })
})

describe('renameStagedShell', () => {
    it('renames the exported name to the name the server reads', () => {
        const dir = makeDir()
        fs.writeFileSync(path.join(dir, EXPORTED_SHELL), '<html>shell</html>')

        renameStagedShell(dir)

        expect(fs.readFileSync(path.join(dir, SERVED_SHELL), 'utf8')).toBe('<html>shell</html>')
        expect(fs.existsSync(path.join(dir, EXPORTED_SHELL))).toBe(false)
    })

    it('is a no-op when the staged dir already holds the served name', () => {
        const dir = makeDir()
        fs.writeFileSync(path.join(dir, SERVED_SHELL), '<html>already</html>')

        expect(() => renameStagedShell(dir)).not.toThrow()
        expect(fs.readFileSync(path.join(dir, SERVED_SHELL), 'utf8')).toBe('<html>already</html>')
    })

    // The regression that shipped a binary serving / but 404ing every deep link:
    // staging copied dist verbatim, so no shell ever landed under app.html.
    it('throws rather than leaving a staged bundle with no shell', () => {
        expect(() => renameStagedShell(makeDir())).toThrow(/no SPA shell/)
    })
})
