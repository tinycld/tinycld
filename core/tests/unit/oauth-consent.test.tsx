// @vitest-environment happy-dom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { ScopeList } from '../../components/oauth/ScopeList'

afterEach(cleanup)

const labels = {
    'notes:read': 'Read your notes',
    'tasks:write': 'Create and modify your tasks',
}

describe('ScopeList', () => {
    it('renders the server-provided description for each scope', () => {
        render(<ScopeList scopes={['notes:read', 'tasks:write']} labels={labels} />)
        expect(screen.getByText('Read your notes')).toBeTruthy()
        expect(screen.getByText('Create and modify your tasks')).toBeTruthy()
    })

    it('falls back to the raw scope name when the server sent no label', () => {
        // Showing the raw name is honest; hiding it would understate what is
        // granted.
        render(<ScopeList scopes={['future:capability']} labels={labels} />)
        expect(screen.getByText('future:capability')).toBeTruthy()
    })

    it('renders nothing when there are no scopes', () => {
        const { container } = render(<ScopeList scopes={[]} labels={labels} />)
        expect(container.firstChild).toBeNull()
    })
})
