import { describe, expect, it } from 'vitest'
import { draftToRecord, recordToDraft } from '../draft'

describe('trigger config', () => {
    it('keeps the form a web form rule names', () => {
        const record = {
            id: 'r1',
            name: 'x',
            scope: 'org',
            owner: 'u',
            trigger: 'core:form-submitted',
            trigger_config: { form: 'f1' },
            conditions: { match: 'all', groups: [] },
            actions: [],
            enabled: true,
            order: 0,
            stop_processing: false,
            created: '',
            updated: '',
        }
        const draft = recordToDraft(record as Parameters<typeof recordToDraft>[0])
        expect(draft.triggerConfig).toEqual({ form: 'f1' })
        expect(draftToRecord(draft).trigger_config).toEqual({ form: 'f1' })
    })
})
