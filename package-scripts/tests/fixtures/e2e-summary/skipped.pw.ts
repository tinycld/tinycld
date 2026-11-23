import { test } from '@playwright/test'

test.skip('is skipped where it is declared', () => {})

test('is skipped while it runs', () => {
    test.skip(true, 'a prerequisite is missing')
})
