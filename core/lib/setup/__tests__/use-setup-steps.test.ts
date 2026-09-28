import { log } from '@tinycld/core/lib/logger'
import { memo } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { isSetupStepModule, loadSetupSteps } from '../use-setup-steps'

const Step = () => null

afterEach(() => {
    vi.restoreAllMocks()
})

describe('isSetupStepModule', () => {
    it('accepts a function or memo component as default', () => {
        expect(isSetupStepModule({ default: Step })).toBe(true)
        expect(isSetupStepModule({ default: memo(Step) })).toBe(true)
    })

    it('rejects a module without a component, or with a hook that is not a function', () => {
        expect(isSetupStepModule(null)).toBe(false)
        expect(isSetupStepModule({})).toBe(false)
        expect(isSetupStepModule({ default: 'Step' })).toBe(false)
        expect(isSetupStepModule({ default: Step, useIsStepDone: true })).toBe(false)
    })
})

describe('loadSetupSteps', () => {
    it('skips and logs a malformed step module, keeping the others', async () => {
        const warn = vi.spyOn(log, 'warn').mockImplementation(() => undefined)
        const steps = await loadSetupSteps([
            { id: 'widgets:broken', label: 'Broken', order: 'a0', load: async () => ({}) },
            {
                id: 'widgets:address',
                label: 'Address',
                order: 'a1',
                load: async () => ({ default: Step }),
            },
        ])
        expect(steps.map(s => s.id)).toEqual(['widgets:address'])
        expect(steps[0].Component).toBe(Step)
        expect(steps[0].useIsStepDone()).toBeNull()
        expect(steps[0].useIsStepVisible()).toBe(true)
        expect(warn).toHaveBeenCalledWith('setup.steps', expect.any(String), {
            stepId: 'widgets:broken',
        })
    })
})
