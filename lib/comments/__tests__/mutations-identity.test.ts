// @vitest-environment happy-dom
import { expect, test } from 'vitest'
import { resolveAuthorFields } from '../mutations'

test('resolveAuthorFields snapshots name and uses user_org id as author', () => {
    expect(
        resolveAuthorFields({ userOrgId: 'uo9', displayName: 'Grace', email: 'g@x.io' })
    ).toEqual({ author: 'uo9', author_name: 'Grace' })
})

test('resolveAuthorFields falls back name → email → Anonymous', () => {
    expect(
        resolveAuthorFields({ userOrgId: 'uo9', displayName: '', email: 'g@x.io' }).author_name
    ).toBe('g@x.io')
    expect(resolveAuthorFields({ userOrgId: 'uo9', displayName: '', email: '' }).author_name).toBe(
        'Anonymous'
    )
})
