// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { ReleaseManifest } from '../../lib/use-release-manifest'

// Mock the data hook so the render never needs a QueryClientProvider or a
// live server — the point of these tests is how AboutSection presents the
// manifest, not how it's fetched.
const useReleaseManifest = vi.fn<() => { data: ReleaseManifest | undefined }>(() => ({
    data: undefined,
}))
vi.mock('@tinycld/core/lib/use-release-manifest', () => ({
    useReleaseManifest: () => useReleaseManifest(),
}))

// Keep the version/server rows from reaching real config/server state.
vi.mock('@tinycld/core/lib/core-config', () => ({ getCoreConfigOptional: () => null }))
vi.mock('@tinycld/core/lib/server-address', () => ({ getResolvedAddress: () => null }))

// expo-constants pulls in expo-modules-core, whose load-time side effects
// (global __DEV__, native TurboModules) crash under Node. Stub it to the one
// field AboutSection reads.
vi.mock('expo-constants', () => ({ default: { expoConfig: { version: '1.0.0' } } }))

// expo-application is a native module (no value under Node). Stub the build
// number; individual tests override it via the mock to cover null (web/dev).
const nativeBuildVersion = vi.hoisted(() => ({ current: '48' as string | null }))
vi.mock('expo-application', () => ({
    get nativeBuildVersion() {
        return nativeBuildVersion.current
    },
}))

import { AboutSection } from '../../components/settings/AboutSection'

afterEach(() => {
    cleanup()
    useReleaseManifest.mockReset()
    useReleaseManifest.mockReturnValue({ data: undefined })
    nativeBuildVersion.current = '48'
})

const MANIFEST: ReleaseManifest = {
    appTag: 'v0.0.3',
    appSha: 'abcdef0123456789',
    releasedAt: '2026-06-05T12:00:00.000Z',
    members: [
        { name: 'mail', repo: 'tinycld/mail', tag: 'v0.1.0', sha: '1111111aaaa' },
        { name: 'calendar', repo: 'tinycld/calendar', tag: 'v0.2.1', sha: '2222222bbbb' },
    ],
}

describe('AboutSection — version row', () => {
    it('shows version, native build number, and short commit when a build number exists', () => {
        nativeBuildVersion.current = '48'
        const { getByText } = render(<AboutSection />)
        // config is mocked null → commit falls back to "dev".slice(0,7) = "dev".
        expect(getByText('1.0.0 (48) · dev')).toBeTruthy()
    })

    it('omits the build number on web/dev where it is null', () => {
        nativeBuildVersion.current = null
        const { getByText } = render(<AboutSection />)
        expect(getByText('1.0.0 (dev)')).toBeTruthy()
    })
})

describe('AboutSection — bundle row', () => {
    // Unit tests run under the react-native web stub (Platform.OS === 'web'), where
    // bundleRowValue() returns null — so the Bundle row is hidden, matching real web
    // behavior (no native updater). The native-rendered value (build-<ts>-ios · hash)
    // is exercised by the OTA e2e on a real Release sim, not here.
    it('omits the Bundle row on web (no native updater)', () => {
        const { queryByText } = render(<AboutSection />)
        expect(queryByText('Bundle')).toBeNull()
    })
})

describe('AboutSection — included packages', () => {
    it('lists each package with its version and short SHA when a manifest is present', () => {
        useReleaseManifest.mockReturnValue({ data: MANIFEST })
        const { getByText } = render(<AboutSection />)

        expect(getByText('Included packages')).toBeTruthy()
        expect(getByText('mail')).toBeTruthy()
        expect(getByText('0.1.0 (1111111)')).toBeTruthy()
        expect(getByText('calendar')).toBeTruthy()
        expect(getByText('0.2.1 (2222222)')).toBeTruthy()
    })

    it('hides the section entirely when the manifest has no members', () => {
        useReleaseManifest.mockReturnValue({ data: { members: [] } })
        const { queryByText } = render(<AboutSection />)
        expect(queryByText('Included packages')).toBeNull()
    })

    it('hides the section while the manifest is still loading (undefined data)', () => {
        useReleaseManifest.mockReturnValue({ data: undefined })
        const { queryByText } = render(<AboutSection />)
        expect(queryByText('Included packages')).toBeNull()
    })
})
