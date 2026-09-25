import { describe, expect, it } from 'vitest'
import { buildPreviewModel } from '../use-workspace-preview'

const base = {
    orgName: '',
    draftName: null,
    logoUrl: '',
    apps: [],
    memberInitials: [],
    isMailOn: false,
}

describe('buildPreviewModel', () => {
    it('prefers the name being typed over the saved one', () => {
        const m = buildPreviewModel({ ...base, orgName: 'tinycld', draftName: 'Harbor Dental' })
        expect(m.name).toBe('Harbor Dental')
        expect(m.initial).toBe('H')
    })
    it('is empty with no name, apps or members', () => {
        expect(buildPreviewModel(base).isEmpty).toBe(true)
        expect(buildPreviewModel({ ...base, memberInitials: ['DR'] }).isEmpty).toBe(false)
    })
})
