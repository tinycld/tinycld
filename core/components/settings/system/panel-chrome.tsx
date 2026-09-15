// Shared chrome for the system-settings panels: the surface they sit on, the
// write-only secret input, and the save row. Extracted from the old
// SetupDashboard Settings tab when these panels moved to the in-app settings
// area, so each panel could become its own route without duplicating the parts
// they share.

import { Button, ButtonText } from '@tinycld/core/ui/button'
import { TextInput } from '@tinycld/core/ui/form'
import type { ReactNode } from 'react'
import type { Control, FieldValues, Path } from 'react-hook-form'
import { Text, View } from 'react-native'
import { SectionLabel } from '../../setup/console-ui'
import type { SettingRow } from '../../setup/system-settings-logic'

export function Panel({ label, children }: { label: string; children: ReactNode }) {
    return (
        <View className="gap-4 p-5 rounded-2xl bg-surface-secondary border border-border">
            <SectionLabel>{label}</SectionLabel>
            {children}
        </View>
    )
}

/** Lead-in copy under a panel's label. */
export function PanelIntro({ children }: { children: ReactNode }) {
    return (
        <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
            {children}
        </Text>
    )
}

// SecretField renders a write-only input for a secret system setting. It NEVER
// seeds the stored value into the field (the value would otherwise round-trip
// through the DOM); instead it shows whether a value is configured and a hint
// that leaving it blank keeps the current one. Submitting blank means "unchanged"
// — the caller must skip the write when the field is empty.
export function SecretField<T extends FieldValues>({
    control,
    name,
    label,
    existing,
}: {
    control: Control<T>
    name: Path<T>
    label: string
    existing: SettingRow | undefined
}) {
    const configured = Boolean(existing?.value)
    return (
        <View className="gap-1.5">
            <TextInput
                control={control}
                name={name}
                label={label}
                placeholder={configured ? '•••••••• (configured)' : 'Not set'}
                secureTextEntry
                autoCapitalize="none"
                hint={
                    configured
                        ? 'Configured. Enter a new value to replace it; leave blank to keep it.'
                        : 'Not set.'
                }
            />
        </View>
    )
}

export function SaveRow({
    testID,
    onPress,
    isPending,
    isDisabled,
}: {
    testID: string
    onPress: () => void
    isPending: boolean
    isDisabled: boolean
}) {
    return (
        <View className="flex-row justify-end">
            <Button testID={testID} onPress={onPress} size="sm" isDisabled={isDisabled}>
                <ButtonText>{isPending ? 'Saving…' : 'Save'}</ButtonText>
            </Button>
        </View>
    )
}
