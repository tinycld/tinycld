import { sendNativeMultipart } from '@tinycld/core/lib/native-upload.native'
import { withNativeUploadBodies } from '@tinycld/core/lib/native-upload-body.native'
import { formDataEntries } from '@tinycld/core/lib/streamed-upload'
import { uploadFileFromUri } from '@tinycld/core/lib/upload-file.native'
import { UploadType } from 'expo-file-system'
import { beforeEach, describe, expect, it } from 'vitest'
import { expoFileSystemStub } from '../../../tests/expo-file-system-stub'

const pdf = new Uint8Array([0x25, 0x50, 0x44, 0x46])
const jpg = new Uint8Array([0xff, 0xd8, 0xff])

beforeEach(() => {
    expoFileSystemStub.reset()
    expoFileSystemStub.respondWith({
        status: 200,
        body: '{"id":"x1"}',
        headers: { 'content-type': 'application/json' },
    })
    expoFileSystemStub.writeFile('file:///cache/report.pdf', pdf)
    expoFileSystemStub.writeFile('file:///cache/ImagePicker/7F3A.jpg', jpg)
})

const report = () => uploadFileFromUri('file:///cache/report.pdf', 'report.pdf', 'application/pdf')
const photo = () =>
    uploadFileFromUri('file:///cache/ImagePicker/7F3A.jpg', 'Beach "day".jpg', 'image/jpeg')

function entriesOf(build: (form: FormData) => void) {
    const form = new FormData()
    build(form)
    return formDataEntries(form)
}

describe('sendNativeMultipart', () => {
    it('streams one file plus fields with MULTIPART, PATCH included', async () => {
        const response = await sendNativeMultipart({
            url: 'https://pb.test/api/collections/users/records/u1',
            method: 'PATCH',
            headers: { Authorization: 'token-1' },
            entries: entriesOf(form => {
                form.append('avatar_crop', '{}')
                form.append('avatar', report())
            }),
        })

        expect(response.status).toBe(200)
        expect(await response.json()).toEqual({ id: 'x1' })
        const [upload] = expoFileSystemStub.uploads
        expect(upload.fileUri).toBe('file:///cache/report.pdf')
        expect(upload.options).toMatchObject({
            httpMethod: 'PATCH',
            uploadType: UploadType.MULTIPART,
            headers: { Authorization: 'token-1' },
            fieldName: 'avatar',
            mimeType: 'application/pdf',
            parameters: { avatar_crop: '{}' },
            sessionType: 'foreground',
        })
    })

    it('sends a file named otherwise on disk from a named copy, then removes it', async () => {
        await sendNativeMultipart({
            url: 'https://pb.test/api/x',
            method: 'POST',
            headers: {},
            entries: entriesOf(form => form.append('file', photo())),
        })
        const [upload] = expoFileSystemStub.uploads
        expect(upload.fileName).toBe('Beach _day_.jpg')
        expect(expoFileSystemStub.hasFile(upload.fileUri)).toBe(false)
        expect(expoFileSystemStub.hasFile('file:///cache/ImagePicker/7F3A.jpg')).toBe(true)
    })

    it('writes any other shape to a body file and sends it with its multipart type', async () => {
        await sendNativeMultipart({
            url: 'https://pb.test/api/mail/send',
            method: 'POST',
            headers: { Authorization: 'token-1' },
            entries: entriesOf(form => {
                form.append('json', '{"to":"a@b.c"}')
                form.append('attachments', report())
                form.append('attachments', photo())
            }),
        })

        const [upload] = expoFileSystemStub.uploads
        expect(upload.options.uploadType).toBe(UploadType.BINARY_CONTENT)
        const contentType = upload.options.headers?.['Content-Type'] ?? ''
        expect(contentType).toMatch(/^multipart\/form-data; boundary=/)
        expect(upload.options.headers?.Authorization).toBe('token-1')
        const form = await new Response(upload.bytes, {
            headers: { 'Content-Type': contentType },
        }).formData()
        expect(form.getAll('json')).toEqual(['{"to":"a@b.c"}'])
        const [first, second] = form.getAll('attachments')
        expect(first instanceof File && first.name).toBe('report.pdf')
        expect(second instanceof File && second.name).toBe('Beach "day".jpg')
        expect(second instanceof File && new Uint8Array(await second.arrayBuffer())).toEqual(jpg)
        // The body file lived in a temporary directory that is gone now.
        expect(expoFileSystemStub.hasFile(upload.fileUri)).toBe(false)
    })

    it('reports progress as (loaded, total)', async () => {
        expoFileSystemStub.reportProgressSteps(2)
        const progress: [number, number][] = []
        await sendNativeMultipart({
            url: 'https://pb.test/api/x',
            method: 'POST',
            headers: {},
            entries: entriesOf(form => form.append('file', report())),
            onProgress: (loaded, total) => progress.push([loaded, total]),
        })
        expect(progress).toEqual([
            [2, 4],
            [4, 4],
        ])
    })

    it('rejects with an AbortError when the signal aborts', async () => {
        const controller = new AbortController()
        controller.abort()
        await expect(
            sendNativeMultipart({
                url: 'https://pb.test/api/x',
                method: 'POST',
                headers: {},
                entries: entriesOf(form => form.append('file', report())),
                signal: controller.signal,
            })
        ).rejects.toMatchObject({ name: 'AbortError' })

        const midway = new AbortController()
        const promise = sendNativeMultipart({
            url: 'https://pb.test/api/x',
            method: 'POST',
            headers: {},
            entries: entriesOf(form => form.append('file', report())),
            signal: midway.signal,
            onProgress: () => midway.abort(),
        })
        await expect(promise).rejects.toMatchObject({ name: 'AbortError' })
    })

    it('rejects with a TypeError when the upload cannot be made', async () => {
        const missing = uploadFileFromUri('file:///cache/gone.pdf', 'gone.pdf', 'application/pdf')
        await expect(
            sendNativeMultipart({
                url: 'https://pb.test/api/x',
                method: 'POST',
                headers: {},
                entries: entriesOf(form => form.append('file', missing)),
            })
        ).rejects.toBeInstanceOf(TypeError)
    })

    it('resolves with an HTTP error status for the caller to map', async () => {
        expoFileSystemStub.respondWith({
            status: 400,
            body: '{"message":"Failed to create record."}',
            headers: {},
        })
        const response = await sendNativeMultipart({
            url: 'https://pb.test/api/x',
            method: 'POST',
            headers: {},
            entries: entriesOf(form => form.append('file', report())),
        })
        expect(response.status).toBe(400)
        expect(await response.json()).toEqual({ message: 'Failed to create record.' })
    })
})

describe('withNativeUploadBodies (serverFetch on native)', () => {
    it('sends a FormData with a file on disk through the native uploader', async () => {
        const fetched: unknown[] = []
        const form = new FormData()
        form.append('id', 'x1')
        form.append('file', report())

        const response = await withNativeUploadBodies(async (_url, init) => {
            fetched.push(init)
            return new Response('{}')
        })('https://pb.test/api/collections/drive_items/records', {
            method: 'POST',
            body: form,
            headers: { Authorization: 'token-1' },
        })

        expect(fetched).toEqual([])
        expect(await response.json()).toEqual({ id: 'x1' })
        expect(expoFileSystemStub.uploads[0].options).toMatchObject({
            httpMethod: 'POST',
            uploadType: UploadType.MULTIPART,
            headers: { authorization: 'token-1' },
            parameters: { id: 'x1' },
        })
    })

    it('leaves a body without a file on disk to the platform fetch', async () => {
        const fetched: unknown[] = []
        const form = new FormData()
        form.append('file', new File(['abc'], 'a.txt'))
        const init = { method: 'POST', body: form }

        await withNativeUploadBodies(async (_url, init) => {
            fetched.push(init)
            return new Response('{}')
        })('https://pb.test/api/x', init)

        expect(fetched).toEqual([init])
        expect(expoFileSystemStub.uploads).toEqual([])
    })
})
