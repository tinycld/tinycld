// @vitest-environment happy-dom

import { act, cleanup, fireEvent, render } from '@testing-library/react'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const OFFLINE = "You're offline — changes can't be saved right now"
const UNREACHABLE = "Can't reach the server — changes can't be saved right now"

function renderSave(props: {
    onPress: () => void
    isDisabled?: boolean
    accessibilityHint?: string
}) {
    return render(
        <ServerActionButton testID="save" {...props}>
            <ButtonText>Save</ButtonText>
        </ServerActionButton>
    )
}

// The stub renders accessibilityHint as a plain attribute.
function hintOf(container: HTMLElement) {
    return container.querySelector('[role="button"]')?.getAttribute('accessibilityhint') ?? null
}

describe('ServerActionButton', () => {
    beforeEach(() => useConnectivityStore.setState({ isOnline: true, isServerReachable: true }))
    afterEach(cleanup)

    it('acts like a plain button while changes can be saved', () => {
        const onPress = vi.fn()
        const { getByText, container } = renderSave({
            onPress,
            accessibilityHint: 'Saves the card',
        })
        fireEvent.click(getByText('Save'))
        expect(onPress).toHaveBeenCalledTimes(1)
        expect(hintOf(container)).toBe('Saves the card')
    })

    it('is disabled offline and says why', () => {
        useConnectivityStore.setState({ isOnline: false })
        const onPress = vi.fn()
        const { getByText, container } = renderSave({ onPress })
        fireEvent.click(getByText('Save'))
        expect(onPress).not.toHaveBeenCalled()
        expect(hintOf(container)).toBe(OFFLINE)
    })

    it('is disabled while the server is unreachable and says why', () => {
        useConnectivityStore.setState({ isServerReachable: false })
        const onPress = vi.fn()
        const { getByText, container } = renderSave({ onPress })
        fireEvent.click(getByText('Save'))
        expect(onPress).not.toHaveBeenCalled()
        expect(hintOf(container)).toBe(UNREACHABLE)
    })

    it('comes back when the connection does', () => {
        useConnectivityStore.setState({ isOnline: false })
        const onPress = vi.fn()
        const { getByText } = renderSave({ onPress })
        act(() => useConnectivityStore.setState({ isOnline: true }))
        fireEvent.click(getByText('Save'))
        expect(onPress).toHaveBeenCalledTimes(1)
    })

    it('keeps its own disabled state while online', () => {
        const onPress = vi.fn()
        const { getByText } = renderSave({ onPress, isDisabled: true })
        fireEvent.click(getByText('Save'))
        expect(onPress).not.toHaveBeenCalled()
    })
})
