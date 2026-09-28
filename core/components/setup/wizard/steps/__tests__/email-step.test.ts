import { describe, expect, it } from 'vitest'
import { emailLeadOf, emailStepIsVisible, mailSendingAppsOf } from '../EmailStep'

const P = () => null

const groups = [
    {
        packageName: 'Post',
        pkgSlug: 'post',
        icon: undefined,
        panels: [
            { slug: 'provider', label: 'Provider', Component: P, keyPrefix: 'mail.' },
            { slug: 'other', label: 'Other', Component: P, keyPrefix: 'vapid.' },
        ],
    },
    {
        packageName: 'Notes',
        pkgSlug: 'notes',
        icon: undefined,
        panels: [{ slug: 'plain', label: 'Plain', Component: P }],
    },
]

describe('mailSendingAppsOf', () => {
    it('names the enabled packages that edit mail settings', () => {
        const enabled = [
            { slug: 'post', name: 'Post' },
            { slug: 'notes', name: 'Notes' },
        ]
        expect(mailSendingAppsOf(groups, enabled)).toEqual(['Post'])
    })
    it('leaves out a mail-sending package the Apps step turned off', () => {
        expect(mailSendingAppsOf(groups, [{ slug: 'notes', name: 'Notes' }])).toEqual([])
    })
})

describe('emailLeadOf', () => {
    it('says which apps send through the provider', () => {
        expect(emailLeadOf(['Post'])).toMatch(/Post also sends every message people write/)
        expect(emailLeadOf(['Post', 'Chat'])).toMatch(/Post and Chat also sends/)
    })
    it('asks only how the server sends when no app sends mail', () => {
        expect(emailLeadOf([])).toMatch(/Choose how it sends them\.$/)
    })
})

describe('emailStepIsVisible', () => {
    const owner = { isOwner: true, isManaged: false, isManagedPending: false }
    it('shows the step to an owner who administers mail', () => {
        expect(emailStepIsVisible(owner)).toBe(true)
    })
    it('hides it from anyone but the owner, or when mail is managed', () => {
        expect(emailStepIsVisible({ ...owner, isOwner: false })).toBe(false)
        expect(emailStepIsVisible({ ...owner, isManaged: true })).toBe(false)
    })
    // Until the managed answer arrives an empty list reads as "not managed",
    // so the step would flash in and out on a managed deployment. Unknown,
    // not hidden, so the wizard does not resume past it meanwhile.
    it('is unknown, and so not shown, until the managed answer arrives', () => {
        expect(emailStepIsVisible({ ...owner, isManagedPending: true })).toBeUndefined()
    })
})
