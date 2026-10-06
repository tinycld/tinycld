import {
    bodyEntries,
    type FormDataEntries,
    planStreamedUpload,
    type StreamedUploadRequest,
    uploadResultToResponse,
    withStreamedUploads,
} from '@tinycld/core/lib/streamed-upload'
import { describe, expect, it } from 'vitest'

// A stand-in for an on-disk file the uploader can stream.
class DiskFile {
    constructor(readonly uri: string) {}
}

const isDiskFile = (value: unknown): value is DiskFile => value instanceof DiskFile

describe('planStreamedUpload', () => {
    it('takes one file and string fields', () => {
        const file = new DiskFile('file:///a.pdf')
        const plan = planStreamedUpload(
            [
                ['id', 'x1'],
                ['file', file],
                ['name', 'a.pdf'],
            ],
            isDiskFile
        )
        expect(plan).toEqual({ file, fieldName: 'file', parameters: { id: 'x1', name: 'a.pdf' } })
    })

    it('refuses shapes the native uploader cannot express', () => {
        const a = new DiskFile('file:///a')
        const b = new DiskFile('file:///b')
        expect(planStreamedUpload([['file', a]], isDiskFile)).not.toBeNull()
        expect(planStreamedUpload([['id', 'x']], isDiskFile)).toBeNull()
        expect(
            planStreamedUpload(
                [
                    ['f', a],
                    ['g', b],
                ],
                isDiskFile
            )
        ).toBeNull()
        expect(
            planStreamedUpload(
                [
                    ['f', a],
                    ['tag', '1'],
                    ['tag', '2'],
                ],
                isDiskFile
            )
        ).toBeNull()
        expect(
            planStreamedUpload(
                [
                    ['f', a],
                    ['meta', new Blob(['x'])],
                ],
                isDiskFile
            )
        ).toBeNull()
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
        expect(response.ok).toBe(true)
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
        const streamed: StreamedUploadRequest<DiskFile>[] = []
        const form = new FormData()
        const wrapped = withStreamedUploads(
            async (url, init) => {
                fetched.push({ url, init })
                return new Response('{}')
            },
            {
                isFile: isDiskFile,
                send: async request => {
                    streamed.push(request)
                    return new Response('{"id":"x1"}', { status: 200 })
                },
            },
            // A browser FormData cannot hold a DiskFile, so the test supplies
            // the entries the native FormData would report.
            body => (body === form ? entries : null)
        )
        return { fetched, streamed, form, wrapped }
    }

    it('streams a one-file upload with its fields, headers and signal', async () => {
        const file = new DiskFile('file:///cache/x.pdf')
        const { fetched, streamed, form, wrapped } = setup([
            ['created_by', 'u1'],
            ['file', file],
        ])
        const controller = new AbortController()

        const response = await wrapped('https://pb.test/api/collections/drive_items/records', {
            method: 'post',
            body: form,
            headers: { Authorization: 'token-1', 'Content-Type': 'multipart/form-data' },
            signal: controller.signal,
        })

        expect(fetched).toEqual([])
        expect(await response.json()).toEqual({ id: 'x1' })
        expect(streamed).toEqual([
            {
                url: 'https://pb.test/api/collections/drive_items/records',
                method: 'POST',
                headers: { authorization: 'token-1' },
                signal: controller.signal,
                upload: { file, fieldName: 'file', parameters: { created_by: 'u1' } },
            },
        ])
    })

    it('leaves every other request to the platform fetch', async () => {
        const file = new DiskFile('file:///cache/x.pdf')
        const two = setup([
            ['a', file],
            ['b', new DiskFile('file:///cache/y.pdf')],
        ])
        const init = { method: 'POST', body: two.form }
        await two.wrapped('https://x', init)
        expect(two.fetched[0].init).toBe(init)
        expect(two.streamed).toEqual([])

        const get = setup([['file', file]])
        await get.wrapped('https://x', { method: 'GET', body: get.form })
        expect(get.streamed).toEqual([])

        const plain = setup(null)
        await plain.wrapped('https://x', { method: 'POST', body: '{"a":1}' })
        expect(plain.fetched[0].init?.body).toBe('{"a":1}')
        expect(plain.streamed).toEqual([])
    })
})
