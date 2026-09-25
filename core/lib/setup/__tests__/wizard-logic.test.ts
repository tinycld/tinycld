import { describe, expect, it } from 'vitest'
import type { StepStatus, WizardState } from '../types'
import { parseWizardState, shouldOpenWizard, summarizeWizard } from '../wizard-logic'

const state = (patch: Partial<WizardState> = {}): WizardState => ({
    startedAt: '2026-09-25T00:00:00Z',
    acknowledged: [],
    skipped: [],
    ...patch,
})
const step = (id: string, patch: Partial<StepStatus> = {}): StepStatus => ({
    id,
    label: id,
    isVisible: true,
    isDone: false,
    ...patch,
})

describe('parseWizardState', () => {
    it('returns null for a missing or corrupt row', () => {
        expect(parseWizardState(undefined)).toBeNull()
        expect(parseWizardState('{nope')).toBeNull()
        expect(parseWizardState('{"acknowledged":[]}')).toBeNull()
    })
    it('parses a valid row', () => {
        expect(
            parseWizardState(JSON.stringify(state({ skipped: ['core:email'] })))?.skipped
        ).toEqual(['core:email'])
    })
})

describe('summarizeWizard', () => {
    it('resumes at the first visible step that is neither done nor skipped', () => {
        const summary = summarizeWizard(
            [
                step('core:workspace', { isDone: true }),
                step('core:apps', { isDone: null }),
                step('core:email', { isVisible: false }),
                step('core:team'),
            ],
            state({ skipped: ['core:apps'] })
        )
        expect(summary.nextStepId).toBe('core:team')
        expect(summary.steps.map(s => s.phase)).toEqual(['done', 'skipped', 'todo'])
        expect(summary.total).toBe(3)
        expect(summary.doneCount).toBe(1)
    })
    it('treats an acknowledged step without derived state as done', () => {
        const summary = summarizeWizard(
            [step('core:apps', { isDone: null })],
            state({ acknowledged: ['core:apps'] })
        )
        expect(summary.nextStepId).toBeNull()
    })
    it('is not settled while a visible step is loading', () => {
        expect(summarizeWizard([step('a:b', { isDone: undefined })], state()).isSettled).toBe(false)
    })
})

describe('shouldOpenWizard', () => {
    const open = (role: string | null, s: WizardState | null) =>
        shouldOpenWizard({ role, state: s, isSettled: true })

    it('opens for owners and admins with an active row', () => {
        expect(open('owner', state())).toBe(true)
        expect(open('admin', state())).toBe(true)
    })
    it('never opens for members, guests, or without a row', () => {
        expect(open('member', state())).toBe(false)
        expect(open('guest', state())).toBe(false)
        expect(open('owner', null)).toBe(false)
    })
    it('stays closed once dismissed or completed, or before the role settles', () => {
        expect(open('owner', state({ dismissedAt: 'x' }))).toBe(false)
        expect(open('owner', state({ completedAt: 'x' }))).toBe(false)
        expect(shouldOpenWizard({ role: 'owner', state: state(), isSettled: false })).toBe(false)
    })
})
