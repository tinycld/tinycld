import { describe, expect, it } from 'vitest'
import { HUB_BACK_LABEL, reportBack } from '../report-back'

describe('reportBack', () => {
    it('names the source and returns to it once its title is read', () => {
        expect(reportBack('ready', 'Profit and Loss')).toEqual({
            label: 'Profit and Loss',
            to: 'source',
        })
    })

    it('reads Reports while the title loads and when its read fails, and still returns to the source', () => {
        expect(reportBack('loading', undefined)).toEqual({ label: HUB_BACK_LABEL, to: 'source' })
        expect(reportBack('error', undefined)).toEqual({ label: HUB_BACK_LABEL, to: 'source' })
    })

    it('reads Reports and goes to the hub when the reader cannot view the source', () => {
        expect(reportBack('missing', undefined)).toEqual({ label: HUB_BACK_LABEL, to: 'hub' })
    })

    it('never shows a blank label', () => {
        expect(reportBack('ready', '')).toEqual({ label: HUB_BACK_LABEL, to: 'source' })
        expect(HUB_BACK_LABEL).toBe('Reports')
    })
})
