import type { AutomationDefinitions } from './types'

export const CORE_PKG_SLUG = 'core'

// Core's own trigger/action catalog. Typed loosely (no schema generic): core's
// collections are part of the base Schema, and this module must not import the
// generated pbSchema to stay usable in the generator's node context.
export const CORE_AUTOMATION: AutomationDefinitions = {
    triggers: [
        { id: 'schedule', label: 'On a schedule', synthetic: 'schedule' },
        { id: 'manual', label: 'Run manually', synthetic: 'manual' },
    ],
    actions: [
        {
            id: 'apply-label',
            label: 'Apply label',
            kind: 'record-op',
            collection: 'label_assignments',
            op: {
                type: 'create',
                set: {
                    label: { param: 'label' },
                    record_id: { context: 'record-id' },
                    collection: { context: 'collection' },
                    user: { context: 'owner' },
                },
            },
            params: [{ key: 'label', field: 'label' }],
        },
        {
            id: 'notify',
            label: 'Send me a notification',
            kind: 'native',
            params: [
                { key: 'title', type: 'text' },
                { key: 'body', type: 'text' },
                { key: 'url', type: 'text', label: 'Link (optional)' },
            ],
        },
    ],
}
