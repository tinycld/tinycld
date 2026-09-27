import { describe, expect, it } from 'vitest'
import { emailStepIsVisible, mailPanelsOf } from '../EmailStep'

const P1 = () => null
const P2 = () => null

describe('mailPanelsOf', () => {
    it('keeps only package panels that edit mail settings', () => {
        const panels = mailPanelsOf([
            {
                packageName: 'Post',
                pkgSlug: 'post',
                icon: undefined,
                panels: [
                    { slug: 'provider', label: 'Provider', Component: P1, keyPrefix: 'mail.' },
                    { slug: 'other', label: 'Other', Component: P2, keyPrefix: 'vapid.' },
                ],
            },
            {
                packageName: 'Notes',
                pkgSlug: 'notes',
                icon: undefined,
                panels: [{ slug: 'plain', label: 'Plain', Component: P2 }],
            },
        ])
        expect(panels).toEqual([{ key: 'post:provider', Component: P1 }])
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
