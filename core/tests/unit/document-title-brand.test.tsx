// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import type { ReactNode } from 'react'
import { Platform } from 'react-native'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

// The shared expo-router/head stub renders nothing; this one renders the
// <title> in place so the test can read the text DocumentTitle builds.
vi.mock('expo-router/head', () => ({
    default: ({ children }: { children?: ReactNode }) => children,
}))
vi.mock('../../lib/use-org-info', () => ({
    useOrgInfo: () => ({ org: { id: 'o1', name: 'Acme' } }),
}))

import { DocumentTitle } from '../../components/DocumentTitle'

const originalOS = Platform.OS

beforeEach(() => {
    Platform.OS = 'web'
})

afterEach(() => {
    cleanup()
    Platform.OS = originalOS
})

// React hoists a rendered <title> into document.head.
test('puts the brand first by default', () => {
    render(<DocumentTitle title="Invoice 1001" pkg="Books" />)
    expect(document.title).toBe('TinyCld: Acme — Books — Invoice 1001')
})

test('includeBrand={false} sets exactly the given title', () => {
    render(<DocumentTitle title="Invoice 1001 from Acme" includeBrand={false} />)
    expect(document.title).toBe('Invoice 1001 from Acme')
})
