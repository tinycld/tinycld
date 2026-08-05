// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import type { TextInputProps as RNTextInputProps } from 'react-native'
import { afterEach, describe, expect, it } from 'vitest'

import { TextInput } from '../TextInput'

// Regression guard for the admin org-create bug: Safari autofilled the password
// field with the account email because TextInput silently dropped the
// `autoComplete`/`textContentType` autofill hints (it cherry-picked a fixed
// prop allowlist instead of forwarding them). With the hints forwarded, the
// browser/keychain treats each field correctly.
// Use RN's own prop types so the test matches what TextInput forwards (its
// `autoComplete` is a narrow union, not `string`) — otherwise the harness fails
// to typecheck under CI's stricter tsc.
function Harness(props: Pick<RNTextInputProps, 'autoComplete' | 'textContentType'>) {
    const { control } = useForm({ defaultValues: { field: '' } })
    return <TextInput control={control} name="field" label="Field" {...props} />
}

afterEach(cleanup)

describe('TextInput autofill hints', () => {
    it('forwards autoComplete to the underlying input', () => {
        const { container } = render(<Harness autoComplete="new-password" />)
        const input = container.querySelector('rn-textinput')
        expect(input?.getAttribute('autoComplete')).toBe('new-password')
    })

    it('forwards textContentType to the underlying input', () => {
        const { container } = render(<Harness textContentType="newPassword" />)
        const input = container.querySelector('rn-textinput')
        expect(input?.getAttribute('textContentType')).toBe('newPassword')
    })
})
