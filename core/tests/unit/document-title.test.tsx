// @vitest-environment happy-dom
import { render } from '@testing-library/react'
import { Platform } from 'react-native'
import { afterEach, expect, test, vi } from 'vitest'

// Mock the org lookup so the test never touches the real pbtsdb stack. The
// point of these tests is *whether* DocumentTitle calls into pbtsdb at all,
// not what the lookup returns — so a spy is exactly the right granularity.
const useOrgInfo = vi.fn(() => ({ orgSlug: 'acme', orgId: 'o1', org: { id: 'o1', name: 'Acme' } }))
vi.mock('../../lib/use-org-info', () => ({ useOrgInfo: () => useOrgInfo() }))

import { DocumentTitle } from '../../components/DocumentTitle'

const originalOS = Platform.OS

afterEach(() => {
    Platform.OS = originalOS
    useOrgInfo.mockClear()
})

// Regression test for the iOS boot crash: on the pre-auth /connect screen the
// pbtsdb Provider isn't mounted (MinimalProviders excludes it). DocumentTitle
// renders nothing on native, so it must NOT reach for the pbtsdb-backed org
// lookup either — doing so threw "useStore must be used within the Provider".
test('renders nothing and skips the pbtsdb org lookup on native', () => {
    Platform.OS = 'ios'
    const { container } = render(<DocumentTitle title="Connect" includeOrg={false} />)
    expect(container.firstChild).toBeNull()
    expect(useOrgInfo).not.toHaveBeenCalled()
})

// The web path still drives the tab title, so it must keep calling the org
// lookup — guards against a "fix" that suppresses the hook everywhere and
// silently drops the org segment from the browser tab title.
test('performs the org lookup on web', () => {
    Platform.OS = 'web'
    render(<DocumentTitle title="Inbox" pkg="Mail" />)
    expect(useOrgInfo).toHaveBeenCalled()
})
