// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { Avatar } from '@tinycld/core/components/Avatar'
import { AvatarStack } from '@tinycld/core/components/AvatarStack'
import { afterEach, describe, expect, it } from 'vitest'

afterEach(cleanup)

// One snapshot per variant the four old renderers produced. These are the
// guard against the collapse silently changing a surface nobody opened during
// review. Snapshotting innerHTML captures the rendered geometry and colors,
// which is exactly what must not drift.
describe('Avatar variants', () => {
    it('renders the solid circle (former NameAvatar)', () => {
        const { container } = render(<Avatar name="Ada Lovelace" colorKey="u1" size={40} />)
        expect(container.innerHTML).toMatchSnapshot()
    })

    it('renders the soft squircle (former MemberAvatar)', () => {
        const { container } = render(
            <Avatar
                name="Ada Lovelace"
                email="ada@example.com"
                size={40}
                palette="soft"
                shape="squircle"
            />
        )
        expect(container.innerHTML).toMatchSnapshot()
    })

    it('renders the dimmed soft squircle', () => {
        const { container } = render(
            <Avatar name="Ada Lovelace" size={40} palette="soft" shape="squircle" dimmed />
        )
        expect(container.innerHTML).toMatchSnapshot()
    })

    it('renders the presence stack (former PresenceAvatars inner)', () => {
        const { container } = render(
            <AvatarStack
                items={[
                    { key: '1', name: 'Ada Lovelace', color: '#3b82f6', colorKey: 'u1' },
                    { key: '2', name: 'Grace Hopper', color: '#22c55e', colorKey: 'u2' },
                ]}
                max={4}
                size={24}
                ring="background"
            />
        )
        expect(container.innerHTML).toMatchSnapshot()
    })

    it('renders the watcher stack with overflow (former CardWatchers)', () => {
        const { container } = render(
            <AvatarStack
                items={[
                    { key: '1', name: 'Ada Lovelace', color: '#3b82f6', colorKey: 'u1' },
                    { key: '2', name: 'Grace Hopper', color: '#22c55e', colorKey: 'u2' },
                    { key: '3', name: 'Alan Turing', color: '#a855f7', colorKey: 'u3' },
                ]}
                max={2}
                size={18}
                ring="card"
            />
        )
        expect(container.innerHTML).toMatchSnapshot()
    })
})
