import { describe, expect, it, vi } from 'vitest'
import {
    bodyEntries,
    encodeMultipart,
    type FormDataEntries,
    isNativeFilePart,
    type MultipartReaders,
    withNativeFileParts,
} from './multipart-body'

const pdfBytes = new Uint8Array([0x25, 0x50, 0x44, 0x46, 0x00, 0xff, 0x0d, 0x0a])

const readers: MultipartReaders = {
    readFile: async () => pdfBytes,
    readBlob: async blob => new Uint8Array(await blob.arrayBuffer()),
}

// Parses the payload with the platform's own multipart parser (undici in
// Node), so a malformed boundary, header or CRLF fails here rather than at
// the server.
function parse(body: Uint8Array<ArrayBuffer>, contentType: string) {
    return new Response(body, { headers: { 'Content-Type': contentType } }).formData()
}

// The app compiles against React Native's FormData typings, which have
// getAll() but no get().
function field(form: Awaited<ReturnType<typeof parse>>, name: string): unknown {
    return form.getAll(name)[0]
}

async function fileBytes(value: unknown) {
    if (!(value instanceof Blob)) throw new Error('expected a file part')
    return new Uint8Array(await value.arrayBuffer())
}

describe('isNativeFilePart', () => {
    it('matches the React Native { uri, name, type } literal', () => {
        expect(
            isNativeFilePart({ uri: 'file:///a.pdf', name: 'a.pdf', type: 'application/pdf' })
        ).toBe(true)
        expect(isNativeFilePart({ uri: 'file:///a.pdf' })).toBe(true)
    })

    it('rejects strings, Blobs and other objects', () => {
        expect(isNativeFilePart('file:///a.pdf')).toBe(false)
        expect(isNativeFilePart(new Blob(['x']))).toBe(false)
        expect(isNativeFilePart({ path: '/a.pdf' })).toBe(false)
        expect(isNativeFilePart(null)).toBe(false)
    })
})

describe('bodyEntries', () => {
    it('reads every part, in order, from a FormData body', () => {
        const form = new FormData()
        form.append('a', '1')
        form.append('b', '2')
        expect(bodyEntries(form)).toEqual([
            ['a', '1'],
            ['b', '2'],
        ])
    })

    it('ignores bodies that are not FormData', () => {
        expect(bodyEntries('{"a":1}')).toBeNull()
        expect(bodyEntries(undefined)).toBeNull()
    })
})

describe('encodeMultipart', () => {
    it('encodes text fields and a native file part into a parseable payload', async () => {
        const entries: FormDataEntries = [
            ['id', 'abc123'],
            ['name', 'Q1 Report.pdf'],
            [
                'file',
                { uri: 'file:///cache/x1.pdf', name: 'Q1 Report.pdf', type: 'application/pdf' },
            ],
            ['description', 'héllo — ünïcode'],
        ]
        const { body, contentType } = await encodeMultipart(entries, readers)
        const form = await parse(body, contentType)

        expect(field(form, 'id')).toBe('abc123')
        expect(field(form, 'description')).toBe('héllo — ünïcode')
        const file = field(form, 'file')
        expect(file).toBeInstanceOf(File)
        expect(file instanceof File && file.name).toBe('Q1 Report.pdf')
        expect(file instanceof File && file.type).toBe('application/pdf')
        expect(await fileBytes(file)).toEqual(pdfBytes)
    })

    it('reads the file the part points at', async () => {
        const readFile = vi.fn(async () => pdfBytes)
        await encodeMultipart([['file', { uri: 'file:///cache/x1.pdf' }]], { ...readers, readFile })
        expect(readFile).toHaveBeenCalledWith('file:///cache/x1.pdf')
    })

    it('falls back to the uri file name and a generic type when the part has none', async () => {
        const { body, contentType } = await encodeMultipart(
            [['file', { uri: 'file:///var/cache/photo-77.jpg' }]],
            readers
        )
        const file = field(await parse(body, contentType), 'file')
        expect(file instanceof File && file.name).toBe('photo-77.jpg')
        expect(file instanceof File && file.type).toBe('application/octet-stream')
    })

    it('escapes quotes and line breaks in names so they cannot break the header', async () => {
        const { body, contentType } = await encodeMultipart(
            [['file', { uri: 'file:///x', name: 'a"b\r\nc.txt', type: 'text/plain' }]],
            readers
        )
        const text = new TextDecoder().decode(body)
        expect(text).toContain('filename="a%22b%0D%0Ac.txt"')
        expect(await parse(body, contentType)).toBeInstanceOf(FormData)
    })

    it('keeps Blob parts next to native file parts', async () => {
        const blob = new File(['{"a":1}'], 'data.json', { type: 'application/json' })
        const { body, contentType } = await encodeMultipart(
            [
                ['meta', blob],
                ['file', { uri: 'file:///x.pdf', name: 'x.pdf', type: 'application/pdf' }],
            ],
            readers
        )
        const form = await parse(body, contentType)
        const meta = field(form, 'meta')
        expect(meta instanceof File && meta.name).toBe('data.json')
        expect(new TextDecoder().decode(await fileBytes(meta))).toBe('{"a":1}')
        expect(await fileBytes(field(form, 'file'))).toEqual(pdfBytes)
    })
})

describe('withNativeFileParts', () => {
    function recordingFetch() {
        const calls: { url: RequestInfo | URL; init: RequestInit | undefined }[] = []
        const fetchImpl = async (url: RequestInfo | URL, init?: RequestInit) => {
            calls.push({ url, init })
            return new Response('{}')
        }
        return { calls, fetchImpl }
    }

    it('passes requests without a native file part through untouched', async () => {
        const { calls, fetchImpl } = recordingFetch()
        const wrapped = withNativeFileParts(fetchImpl, readers)
        const form = new FormData()
        form.append('name', 'x')
        form.append('file', new Blob(['abc']), 'a.txt')
        const init = { method: 'POST', body: form }

        await wrapped('/api/a', init)
        await wrapped('/api/b', { method: 'POST', body: '{"a":1}' })
        await wrapped('/api/c')

        expect(calls[0].init).toBe(init)
        expect(calls[1].init?.body).toBe('{"a":1}')
        expect(calls[2].init).toBeUndefined()
    })

    it('sends a FormData with a native file part as an encoded multipart body', async () => {
        const { calls, fetchImpl } = recordingFetch()
        const form = new FormData()
        // A browser FormData cannot hold the React Native literal, so the test
        // supplies the entries React Native's FormData would report for it.
        const nativeEntries: FormDataEntries = [
            ['created_by', 'u1'],
            ['file', { uri: 'file:///cache/x.pdf', name: 'x.pdf', type: 'application/pdf' }],
        ]
        const wrapped = withNativeFileParts(fetchImpl, readers, body =>
            body === form ? nativeEntries : null
        )
        const controller = new AbortController()

        await wrapped('/api/collections/drive_items/records', {
            method: 'POST',
            body: form,
            headers: { Authorization: 'token-1' },
            signal: controller.signal,
        })

        const sent = calls[0].init
        if (!sent || !(sent.body instanceof Uint8Array)) throw new Error('expected an encoded body')
        const headers = new Headers(sent.headers)
        expect(headers.get('Authorization')).toBe('token-1')
        expect(sent.method).toBe('POST')
        expect(sent.signal).toBe(controller.signal)
        const contentType = headers.get('Content-Type') ?? ''
        expect(contentType).toMatch(/^multipart\/form-data; boundary=/)

        const parsed = await parse(new Uint8Array(sent.body), contentType)
        expect(field(parsed, 'created_by')).toBe('u1')
        expect(await fileBytes(field(parsed, 'file'))).toEqual(pdfBytes)
    })
})
