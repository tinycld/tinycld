import { Panel, PanelIntro, SaveRow } from '@tinycld/core/components/settings/system/panel-chrome'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { Switch } from '@tinycld/core/ui/switch'
import type PocketBase from 'pocketbase'
import { Text, View } from 'react-native'
import { formatTarget, KEY_WINDOW, statusLine, windowSchema } from './auto-upgrade-logic'
import { useAutoUpgrade } from './use-auto-upgrade'

const SWITCH_LABEL = 'Automatically upgrade packages when new versions are available'
const formSchema = z.object({ window: windowSchema })

type AutoUpgrade = ReturnType<typeof useAutoUpgrade>

function WindowEditor({ au, isVisible }: { au: AutoUpgrade; isVisible: boolean }) {
    const {
        control,
        handleSubmit,
        setError,
        formState: { errors, isSubmitted, isDirty, isSubmitting },
    } = useForm({
        resolver: zodResolver(formSchema),
        values: { window: au.window },
        mode: 'onChange',
    })
    if (!isVisible) return null
    const onSubmit = handleSubmit(data =>
        au.saveWindow.mutate(
            { key: KEY_WINDOW, value: data.window, isSecret: false },
            {
                onError: err =>
                    setError('window', {
                        message: err instanceof Error ? err.message : 'Failed to save',
                    }),
            }
        )
    )
    return (
        <View className="gap-2">
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />
            <TextInput
                control={control}
                name="window"
                label="Update window (server time)"
                placeholder="02:00-05:00"
                autoCapitalize="none"
                hint="Updates start only inside this window."
            />
            <SaveRow
                testID="autoupgrade-window-save"
                onPress={onSubmit}
                isPending={au.saveWindow.isPending}
                isDisabled={isSubmitting || !isDirty}
            />
        </View>
    )
}

function PauseNotice({ pause }: { pause: AutoUpgrade['pause'] }) {
    if (!pause) return null
    return (
        <View
            testID="autoupgrade-pause"
            className="gap-1 rounded-lg border border-border bg-surface-secondary p-3"
        >
            <Text className="text-sm font-semibold text-foreground">
                Updates paused: {formatTarget(pause.target)} conflicts with installed packages
            </Text>
            <Text className="text-xs text-muted-foreground">{pause.reason}</Text>
        </View>
    )
}

function BlockedRow({
    row,
    onClear,
}: {
    row: AutoUpgrade['blocked'][number]
    onClear: (id: string) => void
}) {
    return (
        <View
            testID={`autoupgrade-blocked-${row.id}`}
            className="flex-row items-center gap-3 rounded-lg border border-border p-3"
        >
            <View className="flex-1 gap-0.5">
                <Text className="text-sm font-semibold text-foreground">
                    Blocked: {formatTarget(row.target)}
                </Text>
                <Text className="text-xs text-muted-foreground">{row.reason}</Text>
            </View>
            <Button
                testID={`autoupgrade-clear-${row.id}`}
                size="sm"
                variant="outline"
                onPress={() => onClear(row.id)}
            >
                <ButtonText>Clear</ButtonText>
            </Button>
        </View>
    )
}

export function AutoUpgradeSection({ pb, isVisible }: { pb: PocketBase; isVisible: boolean }) {
    const au = useAutoUpgrade(pb, isVisible)
    if (!isVisible) return null
    const available = au.status?.available ?? false
    const line = au.status ? statusLine(au.status) : ''
    const blockedRows = au.blocked.map(row => (
        <BlockedRow key={row.id} row={row} onClear={au.clearBlocked} />
    ))
    return (
        <Panel label="Automatic updates">
            <PanelIntro>
                New versions are installed in the update window. A version that fails its health
                check is rolled back and not tried again until you clear it.
            </PanelIntro>
            <View className="flex-row items-center justify-between gap-3">
                <Text className="flex-1 text-sm text-foreground">{SWITCH_LABEL}</Text>
                <Switch
                    testID="autoupgrade-switch"
                    accessibilityLabel={SWITCH_LABEL}
                    value={au.isOn}
                    isDisabled={!available || !au.isReady}
                    onValueChange={au.setOn}
                />
            </View>
            <Text testID="autoupgrade-status" className="text-xs text-muted-foreground">
                {line}
            </Text>
            <WindowEditor au={au} isVisible={available && !au.windowManaged} />
            <PauseNotice pause={au.pause} />
            <View className="gap-2">{blockedRows}</View>
        </Panel>
    )
}
