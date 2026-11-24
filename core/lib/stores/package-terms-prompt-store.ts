import { create } from '@tinycld/core/lib/store'

/**
 * Whether the owner has seen Settings → Package terms since the app loaded.
 * The start page sends an owner with undecided terms there only while this is
 * false, so "Not now" and a press of Home do not send the owner back again.
 * Not persisted: the next app load asks again.
 */
interface PackageTermsPromptState {
    hasPrompted: boolean
    markPrompted: () => void
}

export const usePackageTermsPromptStore = create<PackageTermsPromptState>()(set => ({
    hasPrompted: false,
    markPrompted: () => set(s => (s.hasPrompted ? s : { hasPrompted: true })),
}))
