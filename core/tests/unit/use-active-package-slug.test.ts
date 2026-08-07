// @vitest-environment happy-dom
import { renderHook } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

// Drive usePathname from a mutable module-level value, following the pattern
// in use-close-on-navigate.test.tsx, so each test can simulate a different
// route without touching the router.
let pathname = '/'
vi.mock('expo-router', () => ({ usePathname: () => pathname }))

// Control the installed-package list independently of whatever happens to be
// assembled in this dev workspace, so the test doesn't drift if a sibling is
// added or removed.
vi.mock('@tinycld/core/lib/search/registry', () => ({
    searchPackages: [
        { slug: 'mail', label: 'Mail', icon: 'mail', order: 5, endpoint: '/api/mail/search' },
        {
            slug: 'drive',
            label: 'Drive',
            icon: 'hard-drive',
            order: 12,
            endpoint: '/api/drive/search',
        },
    ],
}))

import { useActivePackageSlug } from '@tinycld/core/lib/search/use-active-package-slug'

afterEach(() => {
    pathname = '/'
})

test('returns the slug when the path starts with an installed package', () => {
    pathname = '/mail/thread-1'
    const { result } = renderHook(() => useActivePackageSlug())
    expect(result.current).toBe('mail')
})

test('returns null when no segment matches an installed package', () => {
    pathname = '/settings/profile'
    const { result } = renderHook(() => useActivePackageSlug())
    expect(result.current).toBeNull()
})

// The scan walks segments left-to-right and returns on the first match, so a
// non-package prefix segment (here 'org-slug', which names no installed
// package) must not stop it from finding the package segment that follows.
test('skips a leading non-package segment to find the package segment', () => {
    pathname = '/org-slug/drive/folder-1'
    const { result } = renderHook(() => useActivePackageSlug())
    expect(result.current).toBe('drive')
})

// If TWO segments each name an installed package, the leftmost one wins —
// pinning this down guards against a future rewrite that scans in reverse.
test('prefers the earliest matching segment when two segments both match', () => {
    pathname = '/mail/drive'
    const { result } = renderHook(() => useActivePackageSlug())
    expect(result.current).toBe('mail')
})

test('returns null for the root path', () => {
    pathname = '/'
    const { result } = renderHook(() => useActivePackageSlug())
    expect(result.current).toBeNull()
})
