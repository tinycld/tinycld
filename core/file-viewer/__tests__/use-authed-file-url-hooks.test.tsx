// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

// The hooks used to return a tokenless URL while the token was loading, and an
// <img> fetched it at once: a 403 for every protected thumbnail on every
// screen load, then a second fetch when the token arrived.

const h = vi.hoisted(() => ({
    isValid: true,
    getToken: vi.fn<() => Promise<string>>(),
}))

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    pb: {
        files: {
            getURL: (
                record: { collectionId: string; id: string },
                fileName: string,
                opts?: { token?: string }
            ) => {
                const base = `/api/files/${record.collectionId}/${record.id}/${fileName}`
                return opts?.token ? `${base}?token=${opts.token}` : base
            },
            getToken: h.getToken,
        },
        authStore: {
            get isValid() {
                return h.isValid
            },
        },
    },
}))

const { useAuthedFileURL, useAuthedThumbnailURL } = await import(
    '@tinycld/core/file-viewer/use-authed-file-url'
)

const source = {
    collectionId: 'mail_messages',
    recordId: 'rec1',
    fileName: 'hippo.jpg',
    displayName: 'hippo.jpg',
    mimeType: 'image/jpeg',
    size: 100,
}

function wrapper({ children }: { children: ReactNode }) {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

afterEach(() => {
    cleanup()
    h.isValid = true
    h.getToken.mockReset()
})

describe('useAuthedThumbnailURL', () => {
    it('returns no URL until the token arrives, then a tokened one', async () => {
        let resolve: (token: string) => void = () => {}
        h.getToken.mockReturnValue(new Promise(r => (resolve = r)))

        const { result } = renderHook(() => useAuthedThumbnailURL(source, '320x180'), { wrapper })
        expect(result.current).toEqual({ url: '', isLoading: true })

        resolve('tok')
        await waitFor(() =>
            expect(result.current.url).toBe(
                '/api/files/mail_messages/rec1/hippo.jpg?token=tok&thumb=320x180'
            )
        )
        expect(result.current.isLoading).toBe(false)
    })

    it('gives a signed-out viewer the bare URL without waiting', () => {
        h.isValid = false
        const { result } = renderHook(() => useAuthedThumbnailURL(source, '320x180'), { wrapper })
        expect(result.current).toEqual({
            url: '/api/files/mail_messages/rec1/hippo.jpg?thumb=320x180',
            isLoading: false,
        })
        expect(h.getToken).not.toHaveBeenCalled()
    })

    it('falls back to the bare URL when the token fetch fails', async () => {
        h.getToken.mockRejectedValue(new Error('offline'))
        const { result } = renderHook(() => useAuthedThumbnailURL(source, '320x180'), { wrapper })
        await waitFor(() =>
            expect(result.current.url).toBe('/api/files/mail_messages/rec1/hippo.jpg?thumb=320x180')
        )
        expect(result.current.isLoading).toBe(false)
    })
})

describe('useAuthedFileURL', () => {
    it('returns no URL until the token arrives', async () => {
        h.getToken.mockResolvedValue('tok')
        const { result } = renderHook(() => useAuthedFileURL(source), { wrapper })
        expect(result.current).toEqual({ url: '', isLoading: true })
        await waitFor(() =>
            expect(result.current.url).toBe('/api/files/mail_messages/rec1/hippo.jpg?token=tok')
        )
    })
})
