import * as webXhr from '@tinycld/core/file-viewer/xhr-form-data'
import { toXhrParts } from '@tinycld/core/file-viewer/xhr-form-data.native'
import { uploadFileFromUri } from '@tinycld/core/lib/upload-file.native'
import { beforeEach, describe, expect, it } from 'vitest'
import { expoFileSystemStub } from '../../../tests/expo-file-system-stub'

beforeEach(() => {
    expoFileSystemStub.reset()
    expoFileSystemStub.writeFile('file:///cache/DocumentPicker/9C2.pdf', new Uint8Array([1, 2]))
})

describe('toXhrParts (native)', () => {
    it("hands a file on disk to React Native's XHR in its { uri, name, type } form", () => {
        const file = uploadFileFromUri(
            'file:///cache/DocumentPicker/9C2.pdf',
            'invoice.pdf',
            'application/pdf'
        )
        const blob = new Blob(['x'])
        expect(
            toXhrParts([
                ['json', '{"to":"a@b.c"}'],
                ['attachments', file],
                ['other', blob],
            ])
        ).toEqual([
            ['json', '{"to":"a@b.c"}'],
            [
                'attachments',
                {
                    uri: 'file:///cache/DocumentPicker/9C2.pdf',
                    name: 'invoice.pdf',
                    type: 'application/pdf',
                },
            ],
            ['other', blob],
        ])
    })
})

describe('toXhrFormData (web)', () => {
    it('sends the FormData as it is', () => {
        const form = new FormData()
        form.append('file', new File(['x'], 'a.txt'))
        expect(webXhr.toXhrFormData(form)).toBe(form)
    })
})
