import { sendFormDataOnce } from '@tinycld/core/file-viewer/send-form-data.native'
import { uploadFileFromUri } from '@tinycld/core/lib/upload-file.native'
import { UploadType } from 'expo-file-system'
import { beforeEach, describe, expect, it } from 'vitest'
import { expoFileSystemStub } from '../../../tests/expo-file-system-stub'

beforeEach(() => {
    expoFileSystemStub.reset()
    expoFileSystemStub.writeFile('file:///cache/a.pdf', new Uint8Array([1, 2, 3, 4]))
})

function formWithFile() {
    const form = new FormData()
    form.append('name', 'a.pdf')
    form.append('file', uploadFileFromUri('file:///cache/a.pdf', 'a.pdf', 'application/pdf'))
    return form
}

describe('sendFormDataOnce (native)', () => {
    it('sends through the native uploader with the auth token and method', async () => {
        expoFileSystemStub.respondWith({
            status: 200,
            body: '{"id":"r1"}',
            headers: {},
        })
        const progress: [number, number][] = []

        const result = await sendFormDataOnce({
            url: 'https://pb.test/api/collections/org_branding/records/b1',
            formData: formWithFile(),
            authToken: 'token-1',
            method: 'PATCH',
            onProgress: (loaded, total) => progress.push([loaded, total]),
        })

        expect(result).toEqual({ status: 200, body: { id: 'r1' }, retryAfter: null })
        expect(progress.at(-1)).toEqual([4, 4])
        expect(expoFileSystemStub.uploads[0].options).toMatchObject({
            httpMethod: 'PATCH',
            uploadType: UploadType.MULTIPART,
            headers: { Authorization: 'token-1' },
            parameters: { name: 'a.pdf' },
        })
    })

    it('resolves an error status with its body and Retry-After, for the retry and error mapping', async () => {
        expoFileSystemStub.respondWith({
            status: 503,
            body: '{"code":"read_only"}',
            headers: { 'Retry-After': '2' },
        })
        const result = await sendFormDataOnce({
            url: 'https://pb.test/api/x',
            formData: formWithFile(),
            authToken: '',
            method: 'POST',
        })
        expect(result).toEqual({ status: 503, body: { code: 'read_only' }, retryAfter: '2' })
        expect(expoFileSystemStub.uploads[0].options.headers).toEqual({})
    })

    it('treats a non-JSON body as empty, as the web path does', async () => {
        expoFileSystemStub.respondWith({ status: 200, body: 'ok', headers: {} })
        const result = await sendFormDataOnce({
            url: 'https://pb.test/api/x',
            formData: formWithFile(),
            authToken: '',
            method: 'POST',
        })
        expect(result.body).toBeNull()
    })

    it('rejects with an AbortError when aborted', async () => {
        const controller = new AbortController()
        controller.abort()
        await expect(
            sendFormDataOnce({
                url: 'https://pb.test/api/x',
                formData: formWithFile(),
                authToken: '',
                method: 'POST',
                signal: controller.signal,
            })
        ).rejects.toMatchObject({ name: 'AbortError' })
    })
})
