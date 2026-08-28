// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { RuleCard } from '../../components/rules/RuleCard'

// A disabled step card must be genuinely inert, not merely faint. Dimming alone
// left THEN's add-action menu clickable before a trigger was chosen, and it
// opened listing nothing — the dead end this card exists to close.
//
// Two mechanisms, asserted separately because they fail independently:
// pointerEvents stops touch/mouse, while the accessibility props take the
// subtree out of the focus order (on web, pointerEvents: none does NOT stop Tab
// reaching a Pressable inside).

afterEach(cleanup)

function card(props: Partial<React.ComponentProps<typeof RuleCard>> = {}) {
    return render(
        <RuleCard title="THEN" {...props}>
            <span data-testid="body">body</span>
        </RuleCard>
    )
}

describe('RuleCard when enabled', () => {
    it('renders its title and children and stays interactive', () => {
        const { container, getByTestId } = card()
        expect(container.textContent).toContain('THEN')
        expect(getByTestId('body')).toBeTruthy()
        expect(container.firstElementChild?.getAttribute('pointerEvents')).toBe('auto')
    })

    it('shows no hint even when one is supplied', () => {
        // The hint explains a disabled state; showing it on a usable card would
        // tell the user to fix something that isn't broken.
        const { container } = card({ disabledHint: 'Choose a trigger first' })
        expect(container.textContent).not.toContain('Choose a trigger first')
    })
})

describe('RuleCard when disabled', () => {
    it('blocks pointer input over the whole card', () => {
        const { container } = card({ isDisabled: true })
        expect(container.firstElementChild?.getAttribute('pointerEvents')).toBe('none')
    })

    it('takes its subtree out of the focus order', () => {
        const { container } = card({ isDisabled: true })
        const root = container.firstElementChild
        expect(root?.getAttribute('importantForAccessibility')).toBe('no-hide-descendants')
        expect(root?.hasAttribute('accessibilityElementsHidden')).toBe(true)
    })

    it('dims without unmounting, so the cards below it do not jump', () => {
        // Unmounting an unusable step was the original behavior; it moved
        // everything underneath the moment a trigger was picked.
        const { container, getByTestId } = card({ isDisabled: true })
        expect(container.firstElementChild?.getAttribute('class')).toContain('opacity-40')
        expect(getByTestId('body')).toBeTruthy()
        expect(container.textContent).toContain('THEN')
    })

    it('states why it is unusable', () => {
        const { container } = card({ isDisabled: true, disabledHint: 'Choose a trigger first' })
        expect(container.textContent).toContain('Choose a trigger first')
    })
})
