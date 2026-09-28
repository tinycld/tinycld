import { StepHeading } from '@tinycld/core/components/setup/wizard/StepHeading'
import {
    type PackageSystemSettingsGroup,
    packageSystemSettings,
} from '@tinycld/core/lib/packages/derive-components'
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'
import { useAccessiblePackages } from '@tinycld/core/lib/use-accessible-packages'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import {
    useIsManagedSettingsPending,
    useIsSettingManaged,
} from '@tinycld/core/lib/use-managed-settings'
import { View } from 'react-native'
import { EmailSendingForm } from './EmailSendingForm'

const MAIL_PREFIX = 'mail.'

// Acknowledged-only: the server treats unset delivery as on, so a stored value
// cannot tell whether anyone set mail up. The owner's Continue does.
//
// Owner-only, like these panels in Settings. Unknown until the managed answer
// arrives: before it, an empty list reads as "not managed", and the step would
// show and then vanish on a deployment whose mail is managed elsewhere.
export function emailStepIsVisible(input: {
    isOwner: boolean
    isManaged: boolean
    isManagedPending: boolean
}): boolean | undefined {
    if (!input.isOwner) return false
    if (input.isManagedPending) return undefined
    return !input.isManaged
}

export function useIsStepVisible() {
    const { isOwner } = useCurrentRole()
    const isManaged = useIsSettingManaged(MAIL_PREFIX)
    const isManagedPending = useIsManagedSettingsPending()
    return emailStepIsVisible({ isOwner, isManaged, isManagedPending })
}

/**
 * The names of the enabled apps that send mail through this provider. A
 * package says so by contributing a system-settings panel for the `mail.`
 * keys, so core learns it without naming any package. `enabled` is the set
 * the Apps step left on: a hidden app sends nothing.
 */
export function mailSendingAppsOf(
    groups: readonly PackageSystemSettingsGroup[],
    enabled: readonly { slug: string; name: string }[]
): string[] {
    const senders = new Set(
        groups
            .filter(g => g.panels.some(p => p.keyPrefix?.startsWith(MAIL_PREFIX)))
            .map(g => g.pkgSlug)
    )
    return enabled.filter(p => senders.has(p.slug)).map(p => p.name)
}

const BASE_LEAD = 'Your server sends invites, password resets, and notifications by email.'

export function emailLeadOf(mailApps: readonly string[]): string {
    if (mailApps.length === 0) return `${BASE_LEAD} Choose how it sends them.`
    return `${BASE_LEAD} ${mailApps.join(' and ')} also sends every message people write through the same provider.`
}

export default function EmailStep({ next }: SetupStepProps) {
    const isManaged = useIsSettingManaged(MAIL_PREFIX)
    // Until the managed answer arrives an empty list reads as "nothing is
    // managed"; rendering the form then would let an owner act on settings
    // they do not administer.
    const isPending = useIsManagedSettingsPending()
    const mailApps = mailSendingAppsOf(packageSystemSettings, useAccessiblePackages())
    if (isPending || isManaged) return null
    return (
        <View>
            <StepHeading title="Email sending" lead={emailLeadOf(mailApps)} />
            <EmailSendingForm next={next} />
        </View>
    )
}
