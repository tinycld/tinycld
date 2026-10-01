import { describe, expect, it } from 'vitest'
import { collapseSteps } from '../progress-view'

const event = (step: string, message = 'raw', stepProgress?: number) => ({
    step,
    progress: 0,
    message,
    stepProgress,
})

describe('collapseSteps', () => {
    it('merges consecutive events of one step into a single row', () => {
        const rows = collapseSteps(
            [
                event('Building client UI', 'pnpm install'),
                event('Building client UI', 'Progress: resolved 1'),
                event('Building application', 'go build'),
            ],
            'running'
        )
        expect(rows.map(r => r.step)).toEqual(['Building client UI', 'Building application'])
    })

    it('marks only the last row current while running', () => {
        const rows = collapseSteps([event('a'), event('b')], 'running')
        expect(rows.map(r => r.isCurrent)).toEqual([false, true])
        expect(collapseSteps([event('a')], 'success')[0].isCurrent).toBe(false)
    })

    it('keeps the latest step progress of a row', () => {
        const rows = collapseSteps(
            [
                event('Packaging client UI', 'expo export'),
                event('Packaging client UI', '40.0% (800/2000)', 40),
                event('Packaging client UI', '80.0% (1600/2000)', 80),
            ],
            'running'
        )
        expect(rows[0].stepProgress).toBe(80)
    })

    it('has no step progress for a step that cannot measure it', () => {
        expect(collapseSteps([event('Building application')], 'running')[0].stepProgress).toBeNull()
    })

    it('marks a row failed when one of its events failed', () => {
        const rows = collapseSteps(
            [
                event('Building application', 'go build'),
                event('Building application', 'FAILED: go build: exit 1'),
            ],
            'failed'
        )
        expect(rows).toHaveLength(1)
        expect(rows[0].isFailed).toBe(true)
    })
})
