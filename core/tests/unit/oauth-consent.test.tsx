// @vitest-environment happy-dom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { ScopeList } from '../../components/oauth/ScopeList'

afterEach(cleanup)

describe('ScopeList', () => {
    it('renders a human description for each scope', () => {
        render(<ScopeList scopes={['mail:read', 'drive:write']} />)
        expect(screen.getByText('Read your email')).toBeTruthy()
        expect(screen.getByText('Create and modify your files')).toBeTruthy()
    })

    it('falls back to the raw scope name for an unknown scope', () => {
        // A newer server may grant a scope this build has no copy for. Showing
        // the raw name is honest; hiding it would understate what is granted.
        render(<ScopeList scopes={['future:capability']} />)
        expect(screen.getByText('future:capability')).toBeTruthy()
    })

    it('renders nothing when there are no scopes', () => {
        const { container } = render(<ScopeList scopes={[]} />)
        expect(container.firstChild).toBeNull()
    })
})
