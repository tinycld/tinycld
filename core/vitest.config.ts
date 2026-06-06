import { mergeConfig } from 'vitest/config'
import appConfig from '../app/vitest.config'

// Package-scoped vitest for @tinycld/core: inherit the app shell's canonical
// aliases (so @tinycld/core/*, the react-native/expo stubs, etc. resolve
// identically), then scope the run to core's own tests. Core has NO `~/*`
// alias of its own — its tests import via `@tinycld/core/*` (covered by the
// inherited aliases) or relative paths.
//
// Core keeps tests in TWO locations: tests/unit/** (general unit tests) and
// lib/packages/__tests__/** (the runtime package-derivation tests that are the
// heart of the new architecture). Include both.
export default mergeConfig(appConfig, {
    test: {
        root: __dirname,
        include: ['tests/**/*.test.{ts,tsx}', 'lib/**/__tests__/**/*.test.{ts,tsx}'],
    },
})
