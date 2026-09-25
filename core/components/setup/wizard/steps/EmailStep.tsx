import { MailSendingPanel } from '@tinycld/core/components/settings/system/MailSendingPanel'
import {
    type PackageSystemSettingsGroup,
    packageSystemSettings,
} from '@tinycld/core/lib/packages/derive-components'
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import {
    useIsManagedSettingsPending,
    useIsSettingManaged,
} from '@tinycld/core/lib/use-managed-settings'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Suspense } from 'react'
import { Text, View } from 'react-native'

const MAIL_PREFIX = 'mail.'

// Acknowledged-only: the server treats unset delivery as on, so a stored value
// cannot tell whether anyone set mail up. The owner's Continue does.
//
// Owner-only, like these panels in Settings.
export function useIsStepVisible() {
    const { isOwner } = useCurrentRole()
    const isManaged = useIsSettingManaged(MAIL_PREFIX)
    return isOwner && !isManaged
}

/**
 * Package panels that edit mail settings, so a package's provider panel shows
 * here without core naming the package.
 */
export function mailPanelsOf(groups: readonly PackageSystemSettingsGroup[]) {
    return groups.flatMap(g =>
        g.panels
            .filter(p => p.keyPrefix?.startsWith(MAIL_PREFIX))
            .map(p => ({ key: `${g.pkgSlug}:${p.slug}`, Component: p.Component }))
    )
}

const MAIL_PANELS = mailPanelsOf(packageSystemSettings)

export default function EmailStep({ next }: SetupStepProps) {
    const isManaged = useIsSettingManaged(MAIL_PREFIX)
    // Until the managed answer arrives an empty list reads as "nothing is
    // managed"; rendering the panels then would let an owner act on
    // settings they do not administer.
    const isPending = useIsManagedSettingsPending()
    if (isPending || isManaged) return null
    const panels = MAIL_PANELS.map(({ key, Component }) => (
        <Suspense key={key} fallback={null}>
            <Component />
        </Suspense>
    ))
    return (
        <View className="max-w-[440px] gap-1">
            <Text className="text-2xl font-bold text-foreground">Email sending</Text>
            <Text className="mb-3 text-sm text-muted-foreground">
                Your server sends invites and password resets by email. Set up how it sends them.
            </Text>
            <View className="mb-4 gap-4">
                <MailSendingPanel />
                {panels}
            </View>
            <Button className="self-start" onPress={next}>
                <ButtonText>Continue</ButtonText>
            </Button>
        </View>
    )
}
