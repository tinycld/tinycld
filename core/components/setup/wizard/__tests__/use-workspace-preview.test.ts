import { describe, expect, it } from 'vitest'
import { buildPreviewModel, ghostPreviewModel } from '../use-workspace-preview'

const base = {
    orgName: '',
    draftName: null,
    logoUrl: '',
    logoCrop: '',
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
    it('carries the logo crop so a cropped logo renders cropped', () => {
        const m = buildPreviewModel({ ...base, logoUrl: 'https://x/logo.png', logoCrop: '{"x":1}' })
        expect(m.logoCrop).toBe('{"x":1}')
        expect(m.isGhost).toBe(false)
    })
})

describe('ghostPreviewModel', () => {
    it('shows only the owner initials, no org data or apps', () => {
        const m = ghostPreviewModel('DR')
        expect(m).toMatchObject({
            name: '',
            initial: '',
            logoUrl: '',
            logoCrop: '',
            apps: [],
            memberInitials: ['DR'],
            isGhost: true,
            isEmpty: false,
        })
    })
    it('has no avatar before initials are known', () => {
        const m = ghostPreviewModel(undefined)
        expect(m.memberInitials).toEqual([])
        expect(m.isEmpty).toBe(true)
    })
})
