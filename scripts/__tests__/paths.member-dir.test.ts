import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

let root: string

beforeEach(() => {
    root = mkdtempSync(join(tmpdir(), 'tinycld-ws-'))
    // Workspace root with a `tinycld` member that contains app at its root and
    // a nested core/ package.
    const tinycldDir = join(root, 'tinycld')
    mkdirSync(join(tinycldDir, 'scripts'), { recursive: true })
    writeFileSync(join(tinycldDir, 'package.json'), JSON.stringify({ name: 'app' }))
    const coreDir = join(tinycldDir, 'core')
    mkdirSync(coreDir, { recursive: true })
    writeFileSync(join(coreDir, 'package.json'), JSON.stringify({ name: '@tinycld/core' }))
    // A feature sibling at the workspace root (unchanged layout).
    const mailDir = join(root, 'mail')
    mkdirSync(mailDir, { recursive: true })
    writeFileSync(join(mailDir, 'package.json'), JSON.stringify({ name: '@tinycld/mail' }))
    writeFileSync(join(mailDir, 'manifest.ts'), 'export default {}')
    process.env.TINYCLD_APP_DIR = tinycldDir
    vi.resetModules()
})

afterEach(() => {
    delete process.env.TINYCLD_APP_DIR
    rmSync(root, { recursive: true, force: true })
})

test('memberDir resolves @tinycld/core nested inside the tinycld member', async () => {
    const { memberDir } = await import('../paths')
    expect(memberDir('@tinycld/core')).toBe(join(root, 'tinycld', 'core'))
})

test('memberDir resolves a feature sibling at the workspace root', async () => {
    const { memberDir } = await import('../paths')
    expect(memberDir('@tinycld/mail')).toBe(join(root, 'mail'))
})
