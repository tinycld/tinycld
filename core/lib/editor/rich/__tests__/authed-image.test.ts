import { describe, expect, it } from 'vitest'
import { isProtectedFileSrc, resolveProtectedFileSrc } from '../authed-image'

const BASE = 'https://org.example.com'
const SRC = '/api/files/boards_attachments/rec123/photo_abc123.png'

describe('isProtectedFileSrc', () => {
    it('matches a root-relative PocketBase file path', () => {
        expect(isProtectedFileSrc(SRC)).toBe(true)
    })

    it('matches an absolute URL carrying a files path', () => {
        expect(isProtectedFileSrc(`${BASE}${SRC}`)).toBe(true)
    })

    it('rejects data URIs, external URLs, and already-tokened srcs', () => {
        expect(isProtectedFileSrc('data:image/png;base64,AAAA')).toBe(false)
        expect(isProtectedFileSrc('https://example.com/cat.png')).toBe(false)
        expect(isProtectedFileSrc(`${SRC}?token=abc`)).toBe(false)
        expect(isProtectedFileSrc('')).toBe(false)
    })
})

describe('resolveProtectedFileSrc', () => {
    it('absolutizes a relative src against the base and appends the token once', () => {
        const resolved = resolveProtectedFileSrc(SRC, BASE, 'tok1')
        expect(resolved).toBe(`${BASE}${SRC}?token=tok1`)
    })

    it('tokens an absolute same-origin src in place', () => {
        const resolved = resolveProtectedFileSrc(`${BASE}${SRC}`, BASE, 'tok1')
        expect(resolved).toBe(`${BASE}${SRC}?token=tok1`)
    })

    it('absolutizes without a token when none is available yet', () => {
        expect(resolveProtectedFileSrc(SRC, BASE, undefined)).toBe(`${BASE}${SRC}`)
    })

    it('never hands the token to a foreign origin, files-shaped path or not', () => {
        const foreign = `https://evil.example.net${SRC}`
        expect(resolveProtectedFileSrc(foreign, BASE, 'tok1')).toBe(foreign)
    })

    it('leaves non-protected srcs untouched', () => {
        expect(resolveProtectedFileSrc('data:image/png;base64,AAAA', BASE, 'tok1')).toBe(
            'data:image/png;base64,AAAA'
        )
        expect(resolveProtectedFileSrc('https://example.com/cat.png', BASE, 'tok1')).toBe(
            'https://example.com/cat.png'
        )
        expect(resolveProtectedFileSrc(`${SRC}?token=old`, BASE, 'new')).toBe(`${SRC}?token=old`)
    })

    it('keeps a relative src relative when the base URL is itself relative', () => {
        // Web dev serves PocketBase same-origin with pb.baseURL '/' — the
        // browser resolves the path; only the token needs attaching.
        expect(resolveProtectedFileSrc(SRC, '/', 'tok1')).toBe(`${SRC}?token=tok1`)
    })

    it('percent-encodes the token', () => {
        expect(resolveProtectedFileSrc(SRC, '/', 'a+b/c')).toBe(`${SRC}?token=a%2Bb%2Fc`)
    })
})
