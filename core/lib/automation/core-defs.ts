import type { AutomationDefinitions } from './types'

export const CORE_PKG_SLUG = 'core'

// Core's own trigger/action catalog. Typed loosely (no schema generic): core's
// collections are part of the base Schema, and this module must not import the
// generated pbSchema to stay usable in the generator's node context.
export const CORE_AUTOMATION: AutomationDefinitions = {
    triggers: [
        { id: 'schedule', label: 'On a schedule', synthetic: 'schedule' },
        { id: 'manual', label: 'Run manually', synthetic: 'manual' },
        {
            // Onboarding recipes: welcome mail, add to a board, create a
            // contact. Core-owned rather than a package's, because "a person
            // joined" is not any one feature's event.
            //
            // The field list is curated deliberately. `users` carries
            // password and tokenKey, which the engine's exposure rules filter
            // out everywhere — but an allowlist that never names them is the
            // honest way to say what a rule may read, rather than relying on
            // that filter as the only guard.
            //
            // No ownerField: a new user is not "owned" by anyone, so personal
            // rules never match this. Org rules are the sensible scope for
            // onboarding, and they fire regardless of owner resolution.
            id: 'user-added',
            label: 'A user joins',
            collection: 'users',
            on: 'create',
            fields: ['name', 'username', 'email', { key: 'role', label: 'Role' }],
        },
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
        {
            // The outbound half of the integration story: any trigger in any
            // package becomes a source for an external automation tool with
            // no package-specific code. Native IN CORE for send-email's
            // reason — it ships in every build.
            //
            // `secret` is optional; when set the body carries an
            // X-TinyCld-Signature-256 HMAC so the receiver can verify the
            // post came from this deployment. It is a rule param rather than
            // a system setting because each destination has its own.
            id: 'post-webhook',
            label: 'Post to a webhook',
            kind: 'native',
            params: [
                { key: 'url', type: 'text', label: 'URL' },
                { key: 'secret', type: 'text', label: 'Signing secret (optional)' },
            ],
        },
        {
            // The universal "email me / the team when X". Native, but native
            // IN CORE — which is the point: it ships in every build
            // regardless of which feature packages an org installed, so an
            // org without mail (whose build links no mail Go, hence no
            // mail:send-message) still has it.
            //
            // Backed by the same core mailer every transactional email uses,
            // so it inherits the deployment's configured provider rather
            // than introducing a second outbound path.
            id: 'send-email',
            label: 'Send an email',
            kind: 'native',
            params: [
                { key: 'to', type: 'text', label: 'To' },
                { key: 'subject', type: 'text', label: 'Subject' },
                { key: 'body', type: 'text', label: 'Body' },
            ],
        },
    ],
}
