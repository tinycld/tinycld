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
    it('APP_DIR is the tinycld member dir', () => {
        expect(path.basename(APP_DIR)).toBe('tinycld')
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
