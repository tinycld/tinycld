import { describe, expect, it } from 'vitest'
import { isReloadAvailable, ReloadUnavailableError, reloadJsContext } from '../reload-js-context'

// The single most important property of this helper, and the reason it exists:
// a reload that silently no-ops leaves `pb` on the new server while every
// collection stays bound to the old one — a half-switch that presents as
// success. In every environment without a real native reload it must THROW.
//
// The unit environment is one such environment (the react-native stub reports
// Platform.OS === 'web', and the app-updater stub has no reload()), so these
// assertions exercise the refusal path directly.
describe('reloadJsContext', () => {
    it('refuses rather than resolving when no reload mechanism exists', async () => {
        await expect(reloadJsContext()).rejects.toBeInstanceOf(ReloadUnavailableError)
    })

    it('reports itself unavailable so callers can say so up front', () => {
        expect(isReloadAvailable()).toBe(false)
    })

    it('explains why it refused', async () => {
        await expect(reloadJsContext()).rejects.toThrow(/Cannot restart the JS context/)
    })
})
