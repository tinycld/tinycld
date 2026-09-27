import { describe, expect, it } from 'vitest'
import * as EmailStep from '../EmailStep'
import { teamIsDone } from '../TeamStep'
import * as WorkspaceStep from '../WorkspaceStep'

describe('derived step state', () => {
    // Every new server already has a name (PocketBase's "Acme"), so a stored
    // name cannot tell whether anyone chose one.
    it('workspace is done once acknowledged, not from the stored name', () => {
        expect('useIsStepDone' in WorkspaceStep).toBe(false)
    })
    it('team is done once anyone besides the owner exists', () => {
        expect(teamIsDone(1)).toBe(false)
        expect(teamIsDone(2)).toBe(true)
    })
    it('email is done once acknowledged, not from a stored setting', () => {
        expect('useIsStepDone' in EmailStep).toBe(false)
    })
})
