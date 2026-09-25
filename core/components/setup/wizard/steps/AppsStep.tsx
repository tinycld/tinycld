import { and, eq, inArray, not } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { getIcon } from '@tinycld/core/components/workspace/package-icon-map'
import { captureException } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { packageRegistry } from '@tinycld/core/lib/packages/static-registry'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { enabledStatusFor } from '@tinycld/core/lib/setup/set-package-enabled'
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Check } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { type AppChoice, appChoicesOf, CORE_SLUG } from './app-choices'

// Package management and pkg_registry writes are owner-only.
export function useIsStepVisible() {
    return useCurrentRole().isOwner
}

function useAppChoices() {
    const [pkgRegistry] = useStore('pkg_registry')
    // `installed` is included because enabling writes it and the server hook
    // only then corrects a bundled slug back to `bundled`; without it the card
    // would vanish for the length of that round trip.
    const { data: rows = [] } = useLiveQuery(query =>
        query
            .from({ p: pkgRegistry })
            .where(({ p }) =>
                and(
                    inArray(p.status, ['bundled', 'installed', 'disabled']),
                    not(eq(p.slug, CORE_SLUG))
                )
            )
            .select(({ p }) => ({ id: p.id, slug: p.slug, status: p.status }))
    )
    // Hiding an app only flips its status; nothing is rebuilt, so the change
    // applies at once and the preview rail follows the same live rows.
    const toggle = useMutation({
        mutationFn: mutation(function* (choice: AppChoice) {
            yield pkgRegistry.update(choice.id, draft => {
                draft.status = enabledStatusFor(!choice.isOn)
            })
        }),
        onError: err => captureException('setup.apps', err),
    })
    return { choices: appChoicesOf(rows, packageRegistry), toggle: toggle.mutate }
}

const CARD_CLASS = {
    on: 'flex-row items-start gap-2.5 rounded-xl border-[1.5px] border-primary bg-primary/10 p-3',
    off: 'flex-row items-start gap-2.5 rounded-xl border-[1.5px] border-border p-3',
} as const

const CHECK_CLASS = {
    on: 'mt-0.5 size-4 items-center justify-center rounded border-[1.5px] border-primary bg-primary',
    off: 'mt-0.5 size-4 items-center justify-center rounded border-[1.5px] border-border',
} as const

function CheckMark({ isOn }: { isOn: boolean }) {
    const onPrimary = useThemeColor('primary-foreground')
    if (!isOn) return <View className={CHECK_CLASS.off} />
    return (
        <View className={CHECK_CLASS.on}>
            <Check size={11} color={onPrimary} strokeWidth={3} />
        </View>
    )
}

function AppCard({ choice, onToggle }: { choice: AppChoice; onToggle: (c: AppChoice) => void }) {
    const muted = useThemeColor('muted-foreground')
    const Icon = getIcon(choice.icon)
    const state = choice.isOn ? 'on' : 'off'
    return (
        <Pressable
            testID={`setup-app-${choice.slug}`}
            onPress={() => onToggle(choice)}
            accessibilityRole="checkbox"
            accessibilityState={{ checked: choice.isOn }}
            accessibilityLabel={choice.name}
            className={`${CARD_CLASS[state]} min-w-[180px] flex-1 basis-[45%]`}
        >
            <CheckMark isOn={choice.isOn} />
            <View className="flex-1 gap-0.5">
                <View className="flex-row items-center gap-1.5">
                    <Icon size={14} color={muted} />
                    <Text className="text-sm font-bold text-foreground">{choice.name}</Text>
                </View>
                <Text className="text-xs text-muted-foreground" numberOfLines={2}>
                    {choice.description}
                </Text>
            </View>
        </Pressable>
    )
}

export default function AppsStep({ next }: SetupStepProps) {
    const { choices, toggle } = useAppChoices()
    const cards = choices.map(c => <AppCard key={c.slug} choice={c} onToggle={toggle} />)
    return (
        <View className="max-w-[440px] gap-1">
            <Text className="text-2xl font-bold text-foreground">Choose your apps</Text>
            <Text className="mb-3 text-sm text-muted-foreground">
                These apps come with your server. Clear an app to hide it from everyone. You can
                show it again, or add more apps, at any time in Settings → Packages.
            </Text>
            <View className="mb-4 flex-row flex-wrap gap-2">{cards}</View>
            <Button className="self-start" onPress={next}>
                <ButtonText>Continue</ButtonText>
            </Button>
        </View>
    )
}
