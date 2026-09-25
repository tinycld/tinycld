import { describe, expect, it } from 'vitest'
import { buildSetupStepEntries, paramToStepId, stepIdToParam } from '../registry'

const load = () => Promise.resolve({ default: () => null })

describe('buildSetupStepEntries', () => {
    it('merges core and package steps in order with slug-prefixed ids', () => {
        const entries = buildSetupStepEntries(
            [
                { id: 'core:workspace', label: 'Workspace', order: 'a0', load },
                { id: 'core:apps', label: 'Apps', order: 'a1', load },
            ],
            [
                {
                    manifest: { slug: 'acme' },
                    setupSteps: [{ id: 'plan', label: 'Plan', order: 'Zz', load }],
                },
                {
                    manifest: { slug: 'widgets' },
                    setupSteps: [{ id: 'address', label: 'Address', order: 'a0s', load }],
                },
                { manifest: { slug: 'none' } },
            ]
        )
        expect(entries.map(e => e.id)).toEqual([
            'acme:plan',
            'core:workspace',
            'widgets:address',
            'core:apps',
        ])
    })
})

describe('step URL params', () => {
    it('round-trips ids through a URL-safe param', () => {
        expect(stepIdToParam('acme-extra:web-address')).toBe('acme-extra.web-address')
        expect(paramToStepId('acme-extra.web-address')).toBe('acme-extra:web-address')
    })
})
