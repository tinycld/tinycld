import { describe, expect, it } from 'vitest'
import { emailIsDone } from '../EmailStep'
import { teamIsDone } from '../TeamStep'
import { workspaceIsDone } from '../WorkspaceStep'

describe('derived step state', () => {
    it('workspace is done once it has a name', () => {
        expect(workspaceIsDone('')).toBe(false)
        expect(workspaceIsDone('  ')).toBe(false)
        expect(workspaceIsDone('Harbor Dental')).toBe(true)
    })
    it('team is done once anyone besides the owner exists', () => {
        expect(teamIsDone(1)).toBe(false)
        expect(teamIsDone(2)).toBe(true)
    })
    it('email is done once delivery is on', () => {
        expect(emailIsDone(undefined)).toBe(false)
        expect(emailIsDone('true')).toBe(true)
    })
})
