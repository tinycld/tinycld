// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { AvatarStack } from '@tinycld/core/components/AvatarStack'
import { afterEach, describe, expect, it } from 'vitest'

afterEach(cleanup)

const items = [
    { key: 'a', name: 'Ada Lovelace' },
    { key: 'b', name: 'Grace Hopper' },
    { key: 'c', name: 'Alan Turing' },
    { key: 'd', name: 'Katherine Johnson' },
    { key: 'e', name: 'Margaret Hamilton' },
]

describe('AvatarStack', () => {
    it('renders every item when under the limit', () => {
        const { container } = render(
            <AvatarStack items={items.slice(0, 3)} max={4} testID="stack" />
        )
        expect(container.textContent).toContain('AL')
        expect(container.textContent).toContain('GH')
        expect(container.textContent).toContain('AT')
        expect(container.querySelector('[testid="stack-overflow"]')).toBeNull()
    })

    it('caps at max and shows a +N badge for the remainder', () => {
        const { container } = render(<AvatarStack items={items} max={3} testID="stack" />)
        expect(container.querySelector('[testid="stack-overflow"]')).not.toBeNull()
        expect(container.textContent).toContain('+2')
        expect(container.textContent).not.toContain('KJ')
    })

    it('overlaps every avatar after the first', () => {
        const { container } = render(
            <AvatarStack items={items.slice(0, 3)} max={4} size={30} testID="stack" />
        )
        const first = container.querySelector('[testid="stack-item-0"]') as HTMLElement
        const second = container.querySelector('[testid="stack-item-1"]') as HTMLElement
        const third = container.querySelector('[testid="stack-item-2"]') as HTMLElement
        expect(first.style.marginLeft).toBe('0px')
        expect(second.style.marginLeft).toBe('-10px')
        expect(third.style.marginLeft).toBe('-10px')
    })

    it('renders nothing when empty', () => {
        const { container } = render(<AvatarStack items={[]} max={4} testID="stack" />)
        expect(container.querySelector('[testid="stack"]')).toBeNull()
    })
})
