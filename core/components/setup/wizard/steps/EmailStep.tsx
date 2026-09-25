import { MailSendingPanel } from '@tinycld/core/components/settings/system/MailSendingPanel'
import {
    type PackageSystemSettingsGroup,
    packageSystemSettings,
} from '@tinycld/core/lib/packages/derive-components'
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'
import { useIsSettingManaged } from '@tinycld/core/lib/use-managed-settings'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Suspense } from 'react'
import { Text, View } from 'react-native'
import { isDeliverySwitchedOn } from '../../../setup/system-settings-logic'
import { useSystemSettings } from '../../../setup/system-settings-store'

const MAIL_PREFIX = 'mail.'

export function emailIsDone(value: string | undefined): boolean {
    return isDeliverySwitchedOn(value)
}

export function useIsStepDone() {
    const { byKey, isReady } = useSystemSettings()
    return isReady ? emailIsDone(byKey.get('mail.delivery_enabled')?.value) : undefined
}

export function useIsStepVisible() {
    return !useIsSettingManaged(MAIL_PREFIX)
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
