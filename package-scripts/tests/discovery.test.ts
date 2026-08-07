import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { afterAll, afterEach, beforeEach, describe, expect, it } from 'vitest'
import { discover, isAppShellName } from '../src/discovery'

// Every test here builds a sandbox workspace, and TINYCLD_APP_DIR beats any
// fixture (it is discover's first precedence rule). The variable is exported
// for real when tooling runs from a worktree checkout, so it must not leak in.
const inheritedAppDir = process.env.TINYCLD_APP_DIR
beforeEach(() => {
    delete process.env.TINYCLD_APP_DIR
})
afterAll(() => {
    if (inheritedAppDir !== undefined) process.env.TINYCLD_APP_DIR = inheritedAppDir
})

// Build a fake workspace: <ws>/{app,contacts,core} with app named "app".
function makeWs(): string {
    const ws = fs.mkdtempSync(path.join(os.tmpdir(), 'tcld-disc-'))
    fs.writeFileSync(
        path.join(ws, 'package.json'),
        JSON.stringify({ workspaces: ['app', 'contacts', 'core'] })
    )
    for (const [dir, name, manifest] of [
        ['app', 'app', false],
        ['contacts', '@tinycld/contacts', true],
        ['core', '@tinycld/core', false],
    ] as const) {
        fs.mkdirSync(path.join(ws, dir), { recursive: true })
        fs.writeFileSync(path.join(ws, dir, 'package.json'), JSON.stringify({ name }))
        if (manifest) fs.writeFileSync(path.join(ws, dir, 'manifest.ts'), 'export default {}')
    }
    return ws
}

describe('discover', () => {
    let ws: string
    beforeEach(() => {
        ws = makeWs()
    })
    afterEach(() => fs.rmSync(ws, { recursive: true, force: true }))

    it('finds the workspace root from a nested cwd', () => {
        const sub = path.join(ws, 'contacts', 'tinycld', 'contacts')
        fs.mkdirSync(sub, { recursive: true })
        const d = discover(sub)
        expect(d.workspaceRoot).toBe(fs.realpathSync(ws))
    })

    it('identifies the app shell member (name "app")', () => {
        const d = discover(path.join(ws, 'contacts'))
        expect(path.basename(d.appDir)).toBe('app')
    })

    it('recognizes the app shell by either "tinycld" or the legacy "app" name', () => {
        expect(isAppShellName('tinycld')).toBe(true)
        expect(isAppShellName('app')).toBe(true)
        expect(isAppShellName('@tinycld/contacts')).toBe(false)
        expect(isAppShellName(null)).toBe(false)
    })

    it('infers the current package from cwd (feature with manifest.ts)', () => {
        const d = discover(path.join(ws, 'contacts', 'tinycld'))
        expect(d.currentPackage?.name).toBe('@tinycld/contacts')
        expect(d.currentPackage?.kind).toBe('feature')
    })

    it('treats the app shell as a valid scope target (no manifest.ts)', () => {
        const d = discover(path.join(ws, 'app'))
        expect(d.currentPackage?.kind).toBe('app')
    })
})

// A workspace can hold several checkouts of the app repo (git worktrees like
// tinycld-cli-wt beside tinycld/). The scan must not just take the first name
// match — that silently targets the wrong checkout.
describe('discover with multiple app-shell checkouts', () => {
    let ws: string
    beforeEach(() => {
        ws = makeWs()
        // A second checkout of the app repo, plus the node_modules symlink
        // link-members would create for the checkout the workspace is wired to.
        fs.mkdirSync(path.join(ws, 'app-wt', 'core'), { recursive: true })
        fs.writeFileSync(path.join(ws, 'app-wt', 'package.json'), JSON.stringify({ name: 'app' }))
        fs.writeFileSync(
            path.join(ws, 'app-wt', 'core', 'package.json'),
            JSON.stringify({ name: '@tinycld/core' })
        )
        fs.mkdirSync(path.join(ws, 'node_modules', '@tinycld'), { recursive: true })
        fs.symlinkSync(
            path.join(ws, 'app-wt', 'core'),
            path.join(ws, 'node_modules', '@tinycld', 'core')
        )
    })
    afterEach(() => fs.rmSync(ws, { recursive: true, force: true }))

    it('resolves the checkout the core symlink points into, not the first name match', () => {
        const d = discover(path.join(ws, 'contacts'))
        expect(path.basename(d.appDir)).toBe('app-wt')
    })

    it('resolves the checkout cwd is inside, and scopes to it', () => {
        const d = discover(path.join(ws, 'app-wt'))
        expect(path.basename(d.appDir)).toBe('app-wt')
        expect(d.currentPackage?.kind).toBe('app')
    })

    it('honors TINYCLD_APP_DIR above everything', () => {
        process.env.TINYCLD_APP_DIR = path.join(ws, 'app')
        try {
            const d = discover(path.join(ws, 'contacts'))
            expect(path.basename(d.appDir)).toBe('app')
        } finally {
            delete process.env.TINYCLD_APP_DIR
        }
    })
})
