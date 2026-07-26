// @vitest-environment happy-dom
import { expect, test } from 'vitest'
import { resolveAuthorFields } from '../mutations'

test('resolveAuthorFields snapshots name and uses user_org id as author', () => {
    expect(resolveAuthorFields({ userId: 'user9', displayName: 'Grace', email: 'g@x.io' })).toEqual(
        { author: 'user9', author_name: 'Grace' }
    )
})

test('resolveAuthorFields falls back name → email → Anonymous', () => {
    expect(
        resolveAuthorFields({ userId: 'user9', displayName: '', email: 'g@x.io' }).author_name
    ).toBe('g@x.io')
    expect(resolveAuthorFields({ userId: 'user9', displayName: '', email: '' }).author_name).toBe(
        'Anonymous'
    )
})
