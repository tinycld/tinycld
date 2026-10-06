import {
    bodyEntries,
    type FormDataEntries,
    type MultipartUploadRequest,
    planSingleFileUpload,
    requireUploadMethod,
    uploadResultToResponse,
    withStreamedUploads,
    writeMultipart,
} from '@tinycld/core/lib/streamed-upload'
import { describe, expect, it } from 'vitest'

// A stand-in for an on-disk file the uploader streams.
class DiskFile {
    constructor(
        readonly name: string,
        readonly bytes: Uint8Array,
        readonly type = 'application/pdf'
    ) {}
}

const isDiskFile = (value: unknown): value is DiskFile => value instanceof DiskFile

const pdf = new Uint8Array([0x25, 0x50, 0x44, 0x46, 0x00, 0xff, 0x0d, 0x0a])

describe('planSingleFileUpload', () => {
    it('takes one file and string fields', () => {
        const file = new DiskFile('a.pdf', pdf)
        expect(
            planSingleFileUpload(
                [
                    ['id', 'x1'],
                    ['file', file],
                    ['name', 'a.pdf'],
                ],
                isDiskFile
            )
        ).toEqual({ file, fieldName: 'file', parameters: { id: 'x1', name: 'a.pdf' } })
    })

    it('refuses shapes MULTIPART cannot express', () => {
        const a = new DiskFile('a', pdf)
        const b = new DiskFile('b', pdf)
        const shapes: FormDataEntries[] = [
            [['id', 'x']],
            [
                ['f', a],
                ['g', b],
            ],
            [
                ['f', a],
                ['tag', '1'],
                ['tag', '2'],
            ],
            [
                ['f', a],
                ['meta', new Blob(['x'])],
            ],
        ]
        for (const entries of shapes) expect(planSingleFileUpload(entries, isDiskFile)).toBeNull()
    })
})

describe('writeMultipart', () => {
    async function encode(entries: FormDataEntries, chunkBytes = 3) {
        const chunks: Uint8Array[] = []
        const boundary = '----test-boundary'
        await writeMultipart(entries, boundary, bytes => chunks.push(bytes), {
            isFile: isDiskFile,
            fileName: file => file.name,
            fileType: file => file.type,
            copyFile: async (file, write) => {
                for (let i = 0; i < file.bytes.byteLength; i += chunkBytes) {
                    write(file.bytes.slice(i, i + chunkBytes))
                }
            },
            readBlob: async blob => new Uint8Array(await blob.arrayBuffer()),
        })
        const body = new Uint8Array(chunks.reduce((sum, chunk) => sum + chunk.byteLength, 0))
        let offset = 0
        for (const chunk of chunks) {
            body.set(chunk, offset)
            offset += chunk.byteLength
        }
        // Parsed by Node's own multipart parser, so a malformed boundary,
        // header or CRLF fails here rather than at the server.
        return new Response(body, {
            headers: { 'Content-Type': `multipart/form-data; boundary=${boundary}` },
        }).formData()
    }

    async function fileBytes(value: unknown) {
        if (!(value instanceof Blob)) throw new Error('expected a file part')
        return new Uint8Array(await value.arrayBuffer())
    }

    it('writes fields and several files, streaming each file in chunks', async () => {
        const form = await encode([
            ['json', '{"to":"a@b.c","subject":"héllo"}'],
            ['attachments', new DiskFile('Q1 report.pdf', pdf)],
            ['attachments', new DiskFile('photo.jpg', new Uint8Array([9, 8, 7]), 'image/jpeg')],
        ])
        expect(form.getAll('json')).toEqual(['{"to":"a@b.c","subject":"héllo"}'])
        const [first, second] = form.getAll('attachments')
        expect(first instanceof File && first.name).toBe('Q1 report.pdf')
        expect(first instanceof File && first.type).toBe('application/pdf')
        expect(await fileBytes(first)).toEqual(pdf)
        expect(second instanceof File && second.name).toBe('photo.jpg')
        expect(await fileBytes(second)).toEqual(new Uint8Array([9, 8, 7]))
    })

    it('keeps Blob parts and escapes names that would break the header', async () => {
        const form = await encode([
            ['meta', new File(['{"a":1}'], 'data.json', { type: 'application/json' })],
            ['file', new DiskFile('a"b\r\nc.pdf', pdf)],
        ])
        const [meta] = form.getAll('meta')
        expect(meta instanceof File && meta.name).toBe('data.json')
        expect(new TextDecoder().decode(await fileBytes(meta))).toBe('{"a":1}')
        // Written escaped, so the header holds; a parser decodes it back.
        const [file] = form.getAll('file')
        expect(file instanceof File && file.name).toBe('a"b\r\nc.pdf')
    })
})

describe('requireUploadMethod', () => {
    it('accepts the methods an upload can use, in any case', () => {
        expect(requireUploadMethod('post')).toBe('POST')
        expect(requireUploadMethod('PATCH')).toBe('PATCH')
        expect(requireUploadMethod('put')).toBe('PUT')
        expect(() => requireUploadMethod('DELETE')).toThrow(TypeError)
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

describe('uploadResultToResponse', () => {
    it('presents the native result as a fetch Response', async () => {
        const response = uploadResultToResponse(
            { status: 200, body: '{"id":"x1"}', headers: { 'content-type': 'application/json' } },
            'https://pb.test/api/collections/drive_items/records'
        )
        expect(response.status).toBe(200)
        expect(response.headers.get('content-type')).toBe('application/json')
        expect(response.url).toBe('https://pb.test/api/collections/drive_items/records')
        expect(await response.json()).toEqual({ id: 'x1' })
    })

    it('drops the body for statuses that cannot carry one', () => {
        const response = uploadResultToResponse({ status: 204, body: '', headers: {} }, 'https://x')
        expect(response.status).toBe(204)
        expect(response.body).toBeNull()
    })
})

describe('withStreamedUploads', () => {
    function setup(entries: FormDataEntries | null) {
        const fetched: { url: RequestInfo | URL; init: RequestInit | undefined }[] = []
        const sent: MultipartUploadRequest[] = []
        const form = new FormData()
        const wrapped = withStreamedUploads(
            async (url, init) => {
                fetched.push({ url, init })
                return new Response('{}')
            },
            {
                isFile: isDiskFile,
                send: async request => {
                    sent.push(request)
                    return new Response('{"id":"x1"}', { status: 200 })
                },
            },
            // A browser FormData cannot hold a DiskFile, so the test supplies
            // the entries the native FormData would report.
            body => (body === form ? entries : null)
        )
        return { fetched, sent, form, wrapped }
    }

    it('sends any multipart body with a file on disk to the uploader', async () => {
        const entries: FormDataEntries = [
            ['created_by', 'u1'],
            ['file', new DiskFile('x.pdf', pdf)],
            ['file', new DiskFile('y.pdf', pdf)],
        ]
        const { fetched, sent, form, wrapped } = setup(entries)
        const controller = new AbortController()

        const response = await wrapped('https://pb.test/api/collections/drive_items/records', {
            method: 'patch',
            body: form,
            headers: { Authorization: 'token-1', 'Content-Type': 'multipart/form-data' },
            signal: controller.signal,
        })

        expect(fetched).toEqual([])
        expect(await response.json()).toEqual({ id: 'x1' })
        expect(sent).toEqual([
            {
                url: 'https://pb.test/api/collections/drive_items/records',
                method: 'PATCH',
                headers: { authorization: 'token-1' },
                entries,
                signal: controller.signal,
            },
        ])
    })

    it('leaves every other request to the platform fetch', async () => {
        const noFile = setup([['name', 'x']])
        const init = { method: 'POST', body: noFile.form }
        await noFile.wrapped('https://x', init)
        expect(noFile.fetched[0].init).toBe(init)

        const get = setup([['file', new DiskFile('x.pdf', pdf)]])
        await get.wrapped('https://x', { method: 'GET', body: get.form })
        expect(get.sent).toEqual([])

        const plain = setup(null)
        await plain.wrapped('https://x', { method: 'POST', body: '{"a":1}' })
        expect(plain.fetched[0].init?.body).toBe('{"a":1}')
        expect(plain.sent).toEqual([])
    })
})
