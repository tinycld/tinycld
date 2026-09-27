import { describe, expect, it } from 'vitest'
import { enabledStatusFor } from '../set-package-enabled'

describe('enabledStatusFor', () => {
    it('writes installed to enable; the server restores bundled', () => {
        expect(enabledStatusFor(true)).toBe('installed')
    })
    it('writes disabled to disable', () => {
        expect(enabledStatusFor(false)).toBe('disabled')
    })
})
