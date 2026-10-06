import * as webUploadFile from '@tinycld/core/lib/upload-file'
import * as nativeUploadFile from '@tinycld/core/lib/upload-file.native'
import { File as ExpoFile } from 'expo-file-system'
import { beforeEach, describe, expect, it } from 'vitest'
import { expoFileSystemStub } from '../../../tests/expo-file-system-stub'

const bytes = new Uint8Array([1, 2, 3, 4, 5])

beforeEach(() => {
    expoFileSystemStub.reset()
    expoFileSystemStub.writeFile('file:///cache/ImagePicker/7F3A.jpg', bytes)
})

describe('uploadFileFromUri (native)', () => {
    it('wraps the file on disk as an expo-file-system File, which is a Blob', () => {
        const file = nativeUploadFile.uploadFileFromUri(
            'file:///cache/ImagePicker/7F3A.jpg',
            'family.jpg',
            'image/jpeg'
        )
        expect(file).toBeInstanceOf(ExpoFile)
        expect(file).toBeInstanceOf(Blob)
        expect(file instanceof ExpoFile && file.uri).toBe('file:///cache/ImagePicker/7F3A.jpg')
    })

    it('carries the name and type the caller knows, not the ones in the path', () => {
        const file = nativeUploadFile.uploadFileFromUri(
            'file:///cache/ImagePicker/7F3A.jpg',
            'family.jpg',
            'image/heic'
        )
        expect(file.name).toBe('family.jpg')
        expect(file.type).toBe('image/heic')
    })

    it('reads its bytes from the file', async () => {
        const file = nativeUploadFile.uploadFileFromUri(
            'file:///cache/ImagePicker/7F3A.jpg',
            'family.jpg',
            'image/jpeg'
        )
        expect(file.size).toBe(5)
        expect(new Uint8Array(await file.arrayBuffer())).toEqual(bytes)
    })

    it('gives the file URI back for previews', () => {
        const file = nativeUploadFile.uploadFileFromUri(
            'file:///cache/ImagePicker/7F3A.jpg',
            'family.jpg',
            'image/jpeg'
        )
        expect(nativeUploadFile.uploadFileUri(file)).toBe('file:///cache/ImagePicker/7F3A.jpg')
    })
})

describe('upload-file (web)', () => {
    it('refuses to wrap a URI: web pickers already return File objects', () => {
        expect(() => webUploadFile.uploadFileFromUri('blob:x', 'family.jpg', 'image/jpeg')).toThrow(
            /native-only/
        )
    })

    it('gives a blob: URI for previews', () => {
        const file = new File([bytes], 'family.jpg', { type: 'image/jpeg' })
        const uri = webUploadFile.uploadFileUri(file)
        expect(uri).toMatch(/^blob:/)
        URL.revokeObjectURL(uri)
    })
})
