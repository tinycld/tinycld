import { describe, expect, it } from 'vitest'
import type { StepStatus, WizardState } from '../types'
import {
    chosenOrgName,
    continueStep,
    parseWizardState,
    shouldOpenWizard,
    skipStep,
    summarizeWizard,
} from '../wizard-logic'

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
    it('keeps orgNameSeeded, which create-owner --org-name writes', () => {
        expect(
            parseWizardState(JSON.stringify(state({ orgNameSeeded: true })))?.orgNameSeeded
        ).toBe(true)
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
    // Resuming before a step knows whether it shows would skip past it.
    it('is not settled while a step does not yet know if it shows', () => {
        const summary = summarizeWizard(
            [step('a:b', { isVisible: undefined }), step('a:c')],
            state()
        )
        expect(summary.isSettled).toBe(false)
        expect(summary.steps.map(s => s.id)).toEqual(['a:c'])
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

describe('chosenOrgName', () => {
    // PocketBase names every new server "Acme"; that is not the person's choice.
    it('hides the saved name until the workspace step is acknowledged', () => {
        expect(chosenOrgName('Acme', state())).toBe('')
        expect(chosenOrgName('Acme', state({ skipped: ['core:workspace'] }))).toBe('')
    })
    it('shows the saved name once the workspace step is acknowledged', () => {
        expect(chosenOrgName('Harbor Dental', state({ acknowledged: ['core:workspace'] }))).toBe(
            'Harbor Dental'
        )
    })
    it('shows a name the provisioner set with create-owner --org-name', () => {
        expect(chosenOrgName('Harbor Dental', state({ orgNameSeeded: true }))).toBe('Harbor Dental')
    })
    it('shows nothing while the wizard state is unknown', () => {
        expect(chosenOrgName('Acme', null)).toBe('')
    })
})

describe('continueStep', () => {
    it('records a step whose derived state is not done as skipped', () => {
        const next = continueStep(state(), step('widgets:address', { isDone: false }))
        expect(next.skipped).toEqual(['widgets:address'])
        expect(next.acknowledged).toEqual([])
    })
    it('moves resume past a not-done step, exactly as Skip does', () => {
        const statuses = [step('widgets:address'), step('widgets:inbox')]
        const continued = summarizeWizard(statuses, continueStep(state(), statuses[0]))
        const skipped = summarizeWizard(statuses, skipStep(state(), 'widgets:address'))
        expect(continued.nextStepId).toBe('widgets:inbox')
        expect(continued.steps).toEqual(skipped.steps)
        expect(continued.steps[0].phase).toBe('skipped')
    })
    it('records a step whose derived state is still loading as skipped', () => {
        const next = continueStep(state(), step('widgets:address', { isDone: undefined }))
        expect(next.skipped).toEqual(['widgets:address'])
    })
    it('keeps an earlier skip of a step that is still not done', () => {
        const next = continueStep(
            state({ skipped: ['widgets:address'] }),
            step('widgets:address', { isDone: false })
        )
        expect(next.skipped).toEqual(['widgets:address'])
    })
    it('acknowledges a step with no derived state and clears its skip', () => {
        const next = continueStep(
            state({ skipped: ['widgets:intro'] }),
            step('widgets:intro', { isDone: null })
        )
        expect(next.acknowledged).toEqual(['widgets:intro'])
        expect(next.skipped).toEqual([])
    })
    it('acknowledges a done step and clears its skip', () => {
        const next = continueStep(
            state({ skipped: ['widgets:address'] }),
            step('widgets:address', { isDone: true })
        )
        expect(next.acknowledged).toEqual(['widgets:address'])
        expect(next.skipped).toEqual([])
    })
    it('does not add an id twice', () => {
        const once = continueStep(state(), step('widgets:intro', { isDone: null }))
        expect(continueStep(once, step('widgets:intro', { isDone: null })).acknowledged).toEqual([
            'widgets:intro',
        ])
        expect(skipStep(skipStep(state(), 'widgets:a'), 'widgets:a').skipped).toEqual(['widgets:a'])
    })
})
