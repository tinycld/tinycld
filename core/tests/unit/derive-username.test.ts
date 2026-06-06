import { deriveUsername } from '@tinycld/core/lib/derive-username'
import { describe, expect, it } from 'vitest'

describe('deriveUsername', () => {
    // Cases mirror coreserver/users_username_migration_test.go::TestDeriveUsername
    // — the three implementations (Go, JS migration, this TS helper) share that
    // contract, so changes here should propagate to all three.
    it.each([
        ['foo@bar.com', 'foo'],
        ['Bob.Smith+work@example.com', 'bobsmithwork'],
        ['alice123@x.com', 'alice123'],
        ['', 'user'],
        ['@example.com', 'user'],
        ['UPPER@example.com', 'upper'],
        ['dots.and+plus@x.com', 'dotsandplus'],
        ['under_score-dash@x.com', 'under_score-dash'],
        ['noemail', 'noemail'],
        ['ab@x.com', 'ab'],
        ['a@x.com', 'a'],
    ])('deriveUsername(%j) === %j', (input, expected) => {
        expect(deriveUsername(input)).toBe(expected)
    })
})
