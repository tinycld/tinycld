import { describe, expect, it } from 'vitest'
import * as EmailStep from '../EmailStep'
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
    it('email is done once acknowledged, not from a stored setting', () => {
        expect('useIsStepDone' in EmailStep).toBe(false)
    })
})
