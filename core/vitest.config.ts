import { mergeConfig } from 'vitest/config'
import appConfig from '../vitest.config'

// Package-scoped vitest for @tinycld/core: inherit the app shell's canonical
// aliases (so @tinycld/core/*, the react-native/expo stubs, etc. resolve
// identically), then scope the run to core's own tests. Core has NO `~/*`
// alias of its own — its tests import via `@tinycld/core/*` (covered by the
// inherited aliases) or relative paths.
//
// Core colocates tests freely: tests/unit/**, a `__tests__/` dir beside the
// code under test (lib/**, file-viewer/**, ui/**, components/**, …), and the
// occasional loose `<name>.test.ts`. Match all of them anywhere under the
// package root so a new test is never silently skipped for living outside an
// enumerated directory (node_modules etc. are excluded by the inherited config).
export default mergeConfig(appConfig, {
    test: {
        root: __dirname,
        include: ['**/*.test.{ts,tsx}'],
    },
})
