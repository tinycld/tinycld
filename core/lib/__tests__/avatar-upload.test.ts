import { avatarUploadFile } from '@tinycld/core/lib/avatar-upload'
import { describe, expect, it } from 'vitest'

describe('avatarUploadFile', () => {
    it('turns the prepared image into a named upload part of its type', async () => {
        const bytes = new Uint8Array([0x89, 0x50, 0x4e, 0x47])
        const uri = URL.createObjectURL(new Blob([bytes], { type: 'image/png' }))
        const file = await avatarUploadFile({ uri, mimeType: 'image/png' }, 'logo')
        URL.revokeObjectURL(uri)
        expect(file.name).toBe('logo.png')
        expect(file.type).toBe('image/png')
        expect(new Uint8Array(await file.arrayBuffer())).toEqual(bytes)
    })
})
