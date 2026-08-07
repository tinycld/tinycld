import * as fs from 'node:fs'
import * as path from 'node:path'
import { describe, expect, it } from 'vitest'
import {
    APP_DIR,
    GENERATED_DIR,
    PUBLIC_ROUTES_BASE,
    ROUTES_BASE,
    SERVER_DIR,
    WS_ROOT,
} from '../paths'

describe('generator paths', () => {
    // Not asserted by name: APP_DIR is wherever the shell is checked out, and
    // paths.ts honors TINYCLD_APP_DIR precisely so it need not be `tinycld/` —
    // EAS clones the shell into `build/`, and a git worktree gives it whatever
    // name the developer chose. Pinning the basename contradicted that contract
    // and failed in every relocated checkout. What actually matters is that
    // APP_DIR points at a real app shell, which its own marker files prove.
    it('APP_DIR is an app shell dir', () => {
        expect(path.isAbsolute(APP_DIR)).toBe(true)
        expect(fs.existsSync(path.join(APP_DIR, 'app.json'))).toBe(true)
        expect(fs.existsSync(path.join(APP_DIR, 'scripts', 'paths.ts'))).toBe(true)
    })
    it('WS_ROOT is the parent of APP_DIR', () => {
        expect(WS_ROOT).toBe(path.resolve(APP_DIR, '..'))
    })
    it('GENERATED_DIR is app/lib/generated', () => {
        expect(GENERATED_DIR).toBe(path.join(APP_DIR, 'lib', 'generated'))
    })
    // Single-org: the route tree collapsed from app/a/[orgSlug] to the
    // bare app/(app) group. The router owns org multiplexing now, so no
    // org segment appears in an authenticated path.
    it('ROUTES_BASE is app/app/(app)', () => {
        expect(ROUTES_BASE).toBe(path.join(APP_DIR, 'app', '(app)'))
    })
    it('PUBLIC_ROUTES_BASE is app/app/p', () => {
        expect(PUBLIC_ROUTES_BASE).toBe(path.join(APP_DIR, 'app', 'p'))
    })
    it('SERVER_DIR is app/server', () => {
        expect(SERVER_DIR).toBe(path.join(APP_DIR, 'server'))
    })
})
