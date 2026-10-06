import { withNativeUploadBodies } from '@tinycld/core/lib/native-upload-body.native'
import { uploadFileFromUri } from '@tinycld/core/lib/upload-file.native'
import { UploadType } from 'expo-file-system'
import { beforeEach, describe, expect, it } from 'vitest'
import { expoFileSystemStub } from '../../../tests/expo-file-system-stub'

const pdf = new Uint8Array([0x25, 0x50, 0x44, 0x46])

function recordingFetch() {
    const calls: { url: RequestInfo | URL; init: RequestInit | undefined }[] = []
    const fetchImpl = async (url: RequestInfo | URL, init?: RequestInit) => {
        calls.push({ url, init })
        return new Response('{}')
    }
    return { calls, fetchImpl }
}

beforeEach(() => {
    expoFileSystemStub.reset()
    expoFileSystemStub.respondWith({
        status: 200,
        body: '{"id":"x1","file":"report_ab12.pdf"}',
        headers: { 'content-type': 'application/json' },
    })
})

describe('withNativeUploadBodies (native)', () => {
    it('streams a one-file upload from disk with expo-file-system', async () => {
        expoFileSystemStub.writeFile('file:///cache/report.pdf', pdf)
        const { calls, fetchImpl } = recordingFetch()
        const form = new FormData()
        form.append('id', 'x1')
        form.append(
            'file',
            uploadFileFromUri('file:///cache/report.pdf', 'report.pdf', 'application/pdf')
        )

        const response = await withNativeUploadBodies(fetchImpl)(
            'https://pb.test/api/collections/drive_items/records',
            { method: 'POST', body: form, headers: { Authorization: 'token-1' } }
        )

        expect(calls).toEqual([])
        expect(response.status).toBe(200)
        expect(await response.json()).toEqual({ id: 'x1', file: 'report_ab12.pdf' })
        expect(expoFileSystemStub.uploads).toEqual([
            {
                fileUri: 'file:///cache/report.pdf',
                fileName: 'report.pdf',
                url: 'https://pb.test/api/collections/drive_items/records',
                options: {
                    httpMethod: 'POST',
                    uploadType: UploadType.MULTIPART,
                    headers: { authorization: 'token-1' },
                    fieldName: 'file',
                    mimeType: 'application/pdf',
                    parameters: { id: 'x1' },
                    signal: undefined,
                    sessionType: 'foreground',
                },
            },
        ])
    })

    it('sends a file whose name differs from its path from a named copy, then removes it', async () => {
        expoFileSystemStub.writeFile('file:///cache/ImagePicker/7F3A.jpg', pdf)
        const form = new FormData()
        form.append(
            'file',
            uploadFileFromUri('file:///cache/ImagePicker/7F3A.jpg', 'Beach "day".jpg', 'image/jpeg')
        )

        await withNativeUploadBodies(recordingFetch().fetchImpl)('https://pb.test/api/x', {
            method: 'PATCH',
            body: form,
        })

        const [upload] = expoFileSystemStub.uploads
        expect(upload.fileName).toBe('Beach _day_.jpg')
        expect(upload.fileUri).not.toBe('file:///cache/ImagePicker/7F3A.jpg')
        expect(upload.options.httpMethod).toBe('PATCH')
        expect(expoFileSystemStub.hasFile(upload.fileUri)).toBe(false)
        expect(expoFileSystemStub.hasFile('file:///cache/ImagePicker/7F3A.jpg')).toBe(true)
    })

    it('leaves a body without a file on disk to the platform fetch', async () => {
        const { calls, fetchImpl } = recordingFetch()
        const form = new FormData()
        form.append('name', 'x')
        form.append('file', new File(['abc'], 'a.txt'))
        const init = { method: 'POST', body: form }

        await withNativeUploadBodies(fetchImpl)('https://pb.test/api/x', init)

        expect(calls[0].init).toBe(init)
        expect(expoFileSystemStub.uploads).toEqual([])
    })
})
