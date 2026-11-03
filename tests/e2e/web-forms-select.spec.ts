import { expect, test } from '@playwright/test'
import {
    buildAndPublish,
    fillAndSend,
    submissionsNamed,
    visitAnonymously,
} from './web-forms-helpers'

// A select field with no display (the builder's default, "dropdown") is a
// dropdown on the public page: its options stay hidden until the visitor
// opens it. "radio" and "checkbox" keep their own controls.
test('a select field is a dropdown on the public page', async ({ page, browser }) => {
    const url = await buildAndPublish(page, `Select ${Date.now()}`, { withRole: true })
    const visitor = await visitAnonymously(browser, url)
    const role = visitor.getByLabel('Role: Choose…', { exact: true })
    await expect(role).toBeVisible()
    await expect(visitor.getByText('Designer', { exact: true })).toBeHidden()

    await role.click()
    await visitor.getByRole('menuitem', { name: 'Designer' }).click()
    await expect(visitor.getByLabel('Role: Designer', { exact: true })).toBeVisible()

    const name = `Select visitor ${Date.now()}`
    await fillAndSend(visitor, visitor, name)
    await expect(visitor.getByTestId('web-form-confirmation')).toBeVisible()
    await expect
        .poll(async () => (await submissionsNamed(page, name))[0]?.values.role)
        .toBe('design')
})
