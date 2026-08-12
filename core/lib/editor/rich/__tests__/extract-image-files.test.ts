import { describe, expect, it } from 'vitest'
import { extractImageFilesFromDrop, extractImageFilesFromPaste } from '../extract-image-files'

function makeFile(name: string, type: string): File {
    return new File([new Uint8Array(4)], name, { type })
}

describe('extractImageFilesFromDrop', () => {
    it('keeps image files and drops everything else', () => {
        const png = makeFile('a.png', 'image/png')
        const pdf = makeFile('b.pdf', 'application/pdf')
        const event = { dataTransfer: { files: [png, pdf] } }
        expect(extractImageFilesFromDrop(event)).toEqual([png])
    })

    it('returns [] with no dataTransfer or no files', () => {
        expect(extractImageFilesFromDrop({ dataTransfer: null })).toEqual([])
        expect(extractImageFilesFromDrop({ dataTransfer: { files: [] } })).toEqual([])
    })
})

describe('extractImageFilesFromPaste', () => {
    it('keeps image items that yield a file', () => {
        const png = makeFile('a.png', 'image/png')
        const event = {
            clipboardData: {
                items: [
                    { kind: 'file', type: 'image/png', getAsFile: () => png },
                    { kind: 'string', type: 'text/plain', getAsFile: () => null },
                    {
                        kind: 'file',
                        type: 'application/pdf',
                        getAsFile: () => makeFile('b.pdf', 'application/pdf'),
                    },
                ],
            },
        }
        expect(extractImageFilesFromPaste(event)).toEqual([png])
    })

    it('returns [] with no clipboardData', () => {
        expect(extractImageFilesFromPaste({ clipboardData: null })).toEqual([])
    })
})
