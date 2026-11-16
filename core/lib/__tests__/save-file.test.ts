import { fileNameFromDisposition, saveFile } from '@tinycld/core/lib/save-file'
import { describe, expect, it } from 'vitest'

describe('fileNameFromDisposition', () => {
    it.each([
        // The headers core pdfrender.ContentDisposition sends.
        ['attachment; filename="sample-document.pdf"', 'sample-document.pdf'],
        ['attachment; filename="zoo-sales.csv"', 'zoo-sales.csv'],
        [
            'attachment; filename="caf-ltd.pdf"; filename*=UTF-8\'\'Caf%C3%A9%20Ltd.pdf',
            'Café Ltd.pdf',
        ],
        [
            'attachment; filename="document.xlsx"; filename*=UTF-8\'\'%E6%97%A5%E6%9C%AC.xlsx',
            '日本.xlsx',
        ],
        // filename* wins wherever it stands; its charset name is case-blind.
        ['attachment; filename*=utf-8\'\'%C3%A9.pdf; filename="e.pdf"', 'é.pdf'],
        ['attachment; filename=plain.pdf', 'plain.pdf'],
        ['attachment; filename="a \\"quoted\\" name.pdf"', 'a "quoted" name.pdf'],
        // An undecodable or non-UTF-8 filename* falls back to filename.
        ['attachment; filename="x.pdf"; filename*=UTF-8\'\'%E6%97.pdf', 'x.pdf'],
        ['attachment; filename="x.pdf"; filename*=ISO-8859-1\'\'%E9.pdf', 'x.pdf'],
    ])('%j → %j', (header, name) => {
        expect(fileNameFromDisposition(header)).toBe(name)
    })

    it.each([
        [null],
        [''],
        ['attachment'],
        ['attachment; filename=""'],
        ['attachment; filename="../x.pdf"'],
        ['attachment; filename="a\\\\b.pdf"'],
        ['attachment; filename=".."'],
        ['attachment; filename="."'],
        ["attachment; filename*=UTF-8''..%2Fx.pdf"],
        ["attachment; filename*=UTF-8''a%0Ab.pdf"],
    ])('%j has no usable name', header => {
        expect(fileNameFromDisposition(header)).toBeNull()
    })
})

describe('saveFile', () => {
    // A caller can skip fileNameFromDisposition, so saveFile itself refuses a
    // name that could leave the download directory, before any platform branch.
    it.each([['../x.pdf'], ['a/b.pdf'], ['.']])('refuses the unsafe name %j', async fileName => {
        await expect(
            saveFile({
                bytes: new Uint8Array([1, 2, 3]),
                fileName,
                mimeType: 'application/pdf',
            })
        ).rejects.toThrow('Unsafe file name')
    })
})
