import type { PickedFile } from '@tinycld/core/file-viewer/picked-file'
import {
    throttleProgress,
    uploadFormDataWithProgress,
    uploadRecordWithFile,
} from '@tinycld/core/file-viewer/upload-file'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    pb: {
        buildURL: (path: string) => `https://example.test${path}`,
        authStore: { token: 'test-token' },
    },
}))

/**
 * A stand-in for the browser's XMLHttpRequest, capturing what the uploader
 * sent and letting each test drive the response by hand. Only the surface
 * upload-file.ts touches is implemented.
 */
class FakeXHR {
    static last: FakeXHR | null = null

    method = ''
    url = ''
    headers: Record<string, string> = {}
    body: unknown = null
    aborted = false

    status = 200
    response = ''
    responseText = ''

    upload: { onprogress: ((e: ProgressEventLike) => void) | null } = { onprogress: null }
    onload: (() => void) | null = null
    onerror: (() => void) | null = null
    onabort: (() => void) | null = null

    constructor() {
        FakeXHR.last = this
    }

    open(method: string, url: string) {
        this.method = method
        this.url = url
    }

    setRequestHeader(key: string, value: string) {
        this.headers[key] = value
    }

    send(body: unknown) {
        this.body = body
    }

    abort() {
        this.aborted = true
        this.onabort?.()
    }

    /** Completes the request with a JSON body. */
    finish(status: number, payload: unknown) {
        this.status = status
        const text = typeof payload === 'string' ? payload : JSON.stringify(payload)
        this.response = text
        this.responseText = text
        this.onload?.()
    }

    emitProgress(loaded: number, total: number, lengthComputable = true) {
        this.upload.onprogress?.({ loaded, total, lengthComputable })
    }
}

interface ProgressEventLike {
    loaded: number
    total: number
    lengthComputable: boolean
}

const originalXHR = globalThis.XMLHttpRequest

beforeEach(() => {
    FakeXHR.last = null
    globalThis.XMLHttpRequest = FakeXHR as unknown as typeof XMLHttpRequest
})

afterEach(() => {
    globalThis.XMLHttpRequest = originalXHR
    vi.useRealTimers()
})

function pickedFile(name = 'photo.png', size = 2048): PickedFile {
    // A real Blob, because jsdom's FormData rejects anything else. On a device
    // `PickedFile.file` is instead an opaque `{ uri, name, type, size }` that
    // RN's FormData polyfill accepts — the uploader never inspects it either
    // way, so a Blob here exercises the same code path.
    return {
        name,
        type: 'image/png',
        size,
        file: new Blob(['x'.repeat(size)], { type: 'image/png' }) as unknown as File,
    }
}

describe('uploadFormDataWithProgress', () => {
    it('resolves with the parsed body on success', async () => {
        const promise = uploadFormDataWithProgress({
            url: 'https://example.test/api/x',
            formData: new FormData(),
            authToken: 'tok',
        })
        FakeXHR.last?.finish(200, { id: 'abc' })
        await expect(promise).resolves.toEqual({ id: 'abc' })
    })

    it('sends the auth header and defaults to POST', async () => {
        const promise = uploadFormDataWithProgress({
            url: 'https://example.test/api/x',
            formData: new FormData(),
            authToken: 'tok',
        })
        FakeXHR.last?.finish(200, {})
        await promise

        expect(FakeXHR.last?.method).toBe('POST')
        expect(FakeXHR.last?.headers.Authorization).toBe('tok')
    })

    it('honours an explicit method', async () => {
        const promise = uploadFormDataWithProgress({
            url: 'https://example.test/api/x',
            formData: new FormData(),
            authToken: 'tok',
            method: 'PATCH',
        })
        FakeXHR.last?.finish(200, {})
        await promise

        expect(FakeXHR.last?.method).toBe('PATCH')
    })

    it('omits the auth header when there is no token', async () => {
        const promise = uploadFormDataWithProgress({
            url: 'https://example.test/api/x',
            formData: new FormData(),
            authToken: '',
        })
        FakeXHR.last?.finish(200, {})
        await promise

        expect(FakeXHR.last?.headers.Authorization).toBeUndefined()
    })

    it('reports progress only for length-computable events', async () => {
        const onProgress = vi.fn()
        const promise = uploadFormDataWithProgress({
            url: 'https://example.test/api/x',
            formData: new FormData(),
            authToken: 'tok',
            onProgress,
        })

        FakeXHR.last?.emitProgress(10, 100)
        FakeXHR.last?.emitProgress(50, 100, false)
        FakeXHR.last?.finish(200, {})
        await promise

        expect(onProgress).toHaveBeenCalledTimes(1)
        expect(onProgress).toHaveBeenCalledWith(10, 100)
    })

    it('rejects with the server message on an error status', async () => {
        const promise = uploadFormDataWithProgress({
            url: 'https://example.test/api/x',
            formData: new FormData(),
            authToken: 'tok',
        })
        FakeXHR.last?.finish(400, { message: 'Failed to create record.' })
        await expect(promise).rejects.toThrow('Failed to create record.')
    })

    it('falls back to the status code when the error body has no message', async () => {
        const promise = uploadFormDataWithProgress({
            url: 'https://example.test/api/x',
            formData: new FormData(),
            authToken: 'tok',
        })
        FakeXHR.last?.finish(503, 'not json at all')
        await expect(promise).rejects.toThrow('Upload failed (503)')
    })

    it('rejects on a transport error', async () => {
        const promise = uploadFormDataWithProgress({
            url: 'https://example.test/api/x',
            formData: new FormData(),
            authToken: 'tok',
        })
        FakeXHR.last?.onerror?.()
        await expect(promise).rejects.toThrow('Network request failed')
    })

    it('aborts an in-flight request when the signal fires', async () => {
        const controller = new AbortController()
        const promise = uploadFormDataWithProgress({
            url: 'https://example.test/api/x',
            formData: new FormData(),
            authToken: 'tok',
            signal: controller.signal,
        })

        controller.abort()

        expect(FakeXHR.last?.aborted).toBe(true)
        await expect(promise).rejects.toThrow('Aborted')
    })

    it('rejects immediately when handed an already-aborted signal', async () => {
        const controller = new AbortController()
        controller.abort()

        await expect(
            uploadFormDataWithProgress({
                url: 'https://example.test/api/x',
                formData: new FormData(),
                authToken: 'tok',
                signal: controller.signal,
            })
        ).rejects.toThrow('Aborted')

        // Nothing should have been opened at all.
        expect(FakeXHR.last).toBeNull()
    })
})

describe('uploadRecordWithFile', () => {
    it('posts scalar fields and the file to the collection records endpoint', async () => {
        const promise = uploadRecordWithFile({
            collection: 'boards_attachments',
            fields: { id: 'rec1', card: 'card1', size: '2048' },
            file: pickedFile(),
        })
        FakeXHR.last?.finish(200, { id: 'rec1' })
        await promise

        expect(FakeXHR.last?.url).toBe(
            'https://example.test/api/collections/boards_attachments/records'
        )

        const body = FakeXHR.last?.body as FormData
        expect(body.get('id')).toBe('rec1')
        expect(body.get('card')).toBe('card1')
        expect(body.get('size')).toBe('2048')
        expect(body.get('file')).toBeTruthy()
    })

    it('uses a custom file field name when given one', async () => {
        const promise = uploadRecordWithFile({
            collection: 'x',
            fields: {},
            file: pickedFile(),
            fileField: 'avatar',
        })
        FakeXHR.last?.finish(200, {})
        await promise

        const body = FakeXHR.last?.body as FormData
        expect(body.get('avatar')).toBeTruthy()
        expect(body.get('file')).toBeNull()
    })
})

describe('throttleProgress', () => {
    it('drops intermediate writes inside the interval', () => {
        vi.useFakeTimers()
        vi.setSystemTime(0)

        const write = vi.fn()
        const onProgress = throttleProgress(write, 60)

        onProgress(10, 1000)
        onProgress(20, 1000)
        vi.setSystemTime(70)
        onProgress(30, 1000)

        expect(write.mock.calls).toEqual([
            [10, 1000],
            [30, 1000],
        ])
    })

    it('always writes the final byte, however soon it arrives', () => {
        vi.useFakeTimers()
        vi.setSystemTime(0)

        const write = vi.fn()
        const onProgress = throttleProgress(write, 60)

        onProgress(10, 1000)
        onProgress(1000, 1000)

        // Without the final flush the bar would stop short of 100%.
        expect(write).toHaveBeenLastCalledWith(1000, 1000)
    })

    it('always writes the first event, so a bar starts moving at once', () => {
        vi.useFakeTimers()
        vi.setSystemTime(0)

        const write = vi.fn()
        const onProgress = throttleProgress(write, 60)

        onProgress(10, 1000)

        expect(write).toHaveBeenCalledWith(10, 1000)
    })

    it('does not treat an unknown total as final', () => {
        vi.useFakeTimers()
        vi.setSystemTime(0)

        const write = vi.fn()
        const onProgress = throttleProgress(write, 60)

        // The first is written unconditionally; the second is inside the
        // window and `total: 0` must not qualify it as the final byte.
        onProgress(10, 0)
        onProgress(20, 0)

        expect(write).toHaveBeenCalledTimes(1)
        expect(write).toHaveBeenCalledWith(10, 0)
    })
})
