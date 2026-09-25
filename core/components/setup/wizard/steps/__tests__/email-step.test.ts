import { describe, expect, it } from 'vitest'
import { mailPanelsOf } from '../EmailStep'

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
