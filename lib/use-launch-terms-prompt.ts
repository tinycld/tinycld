import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { hasUndecidedTerms } from '@tinycld/core/lib/packages/terms-state'
import { usePackageTermsList } from '@tinycld/core/lib/packages/use-package-terms'
import { usePackageTermsPromptStore } from '@tinycld/core/lib/stores/package-terms-prompt-store'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'

/**
 * Where a launch sends an owner who has not decided on a package's terms: a
 * package added after setup, terms skipped in the wizard, or a new terms
 * version. `href` is null when there is nothing to ask. A declined package does
 * not ask again; Settings → Package terms still lists it. It asks once per app
 * load: Package terms marks the prompt when it opens, so "Not now" and a press
 * of Home land on a package after that.
 *
 * Both launch routes use it: '/' (app/index.tsx), where a sign-in and a cold
 * load land, and the org root (app/a/(app)/index.tsx), where Home goes. A deep
 * link is not interrupted; the package's own route shows its terms.
 */
export function useLaunchTermsPrompt() {
    const { isOwner } = useCurrentRole()
    const terms = usePackageTermsList()
    const hasPrompted = usePackageTermsPromptStore(s => s.hasPrompted)
    const orgHref = useOrgHref()
    const shouldPrompt = !hasPrompted && isOwner && hasUndecidedTerms(terms.pending)
    return {
        isReady: terms.isReady,
        href: shouldPrompt ? orgHref('settings/package-terms') : null,
    }
}
