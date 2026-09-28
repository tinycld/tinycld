import { inArray } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { SetupContinueButton } from '@tinycld/core/components/setup/wizard/SetupContinueButton'
import { StepHeading } from '@tinycld/core/components/setup/wizard/StepHeading'
import { getIcon } from '@tinycld/core/components/workspace/package-icon-map'
import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { captureException, errorToString } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { notify } from '@tinycld/core/lib/notify'
import { packageRegistry } from '@tinycld/core/lib/packages/static-registry'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { enabledStatusFor } from '@tinycld/core/lib/setup/set-package-enabled'
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { Check } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { type AppChoice, appChoicesOf } from './app-choices'

// Package management and pkg_registry writes are owner-only.
export function useIsStepVisible() {
    return useCurrentRole().isOwner
}

function useAppChoices() {
    const [pkgRegistry] = useStore('pkg_registry')
    // `installed` is included because enabling writes it and the server hook
    // only then corrects a bundled slug back to `bundled`; without it the card
    // would vanish for the length of that round trip. The core row is dropped
    // by appChoicesOf, which already skips it, so the query excludes nothing.
    const { data: rows = [] } = useLiveQuery(query =>
        query
            .from({ p: pkgRegistry })
            .where(({ p }) => inArray(p.status, ['bundled', 'installed', 'disabled']))
            .select(({ p }) => ({ id: p.id, slug: p.slug, status: p.status }))
    )
    // Hiding an app only flips its status; nothing is rebuilt, so the change
    // applies at once.
    const toggle = useMutation({
        mutationFn: mutation(function* (choice: AppChoice) {
            yield pkgRegistry.update(choice.id, draft => {
                draft.status = enabledStatusFor(!choice.isOn)
            })
        }),
        onError: err => {
            captureException('setup.apps', err)
            notify.emit({
                event: 'mutation.error',
                title: 'Could not change the app',
                body: errorToString(err),
                data: { operation: 'setup.apps', error: errorToString(err) },
            })
        },
    })
    return { choices: appChoicesOf(rows, packageRegistry), toggle: toggle.mutate }
}

const CARD_CLASS = {
    on: 'flex-row items-center gap-3 rounded-xl border-2 border-primary bg-accent p-3',
    off: 'flex-row items-center gap-3 rounded-xl border-2 border-border bg-background p-3',
} as const

// Two columns where the card has room; one on a phone, so descriptions stay readable.
const CARD_WIDTH_CLASS = {
    wide: 'min-w-[200px] flex-1 basis-[46%]',
    narrow: 'w-full',
} as const

const ICON_BOX_CLASS = {
    on: 'size-10 items-center justify-center rounded-lg bg-primary',
    off: 'size-10 items-center justify-center rounded-lg bg-surface-secondary',
} as const

const CHECK_CLASS = {
    on: 'size-5 items-center justify-center rounded-full bg-primary',
    off: 'size-5 items-center justify-center rounded-full border-2 border-border',
} as const

function CheckMark({ isOn }: { isOn: boolean }) {
    const onPrimary = useThemeColor('primary-foreground')
    if (!isOn) return <View className={CHECK_CLASS.off} />
    return (
        <View className={CHECK_CLASS.on}>
            <Check size={12} color={onPrimary} strokeWidth={3} />
        </View>
    )
}

function AppCard({
    choice,
    widthClass,
    onToggle,
}: {
    choice: AppChoice
    widthClass: string
    onToggle: (c: AppChoice) => void
}) {
    const onPrimary = useThemeColor('primary-foreground')
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
            className={`${CARD_CLASS[state]} ${widthClass}`}
        >
            <View className={ICON_BOX_CLASS[state]}>
                <Icon size={18} color={choice.isOn ? onPrimary : muted} />
            </View>
            <View className="flex-1 gap-0.5">
                <Text className="text-sm font-semibold text-foreground">{choice.name}</Text>
                <Text className="text-xs leading-4 text-muted-foreground" numberOfLines={2}>
                    {choice.description}
                </Text>
            </View>
            <CheckMark isOn={choice.isOn} />
        </Pressable>
    )
}

export default function AppsStep({ next }: SetupStepProps) {
    const { choices, toggle } = useAppChoices()
    const widthClass =
        useBreakpoint() === 'mobile' ? CARD_WIDTH_CLASS.narrow : CARD_WIDTH_CLASS.wide
    const cards = choices.map(c => (
        <AppCard key={c.slug} choice={c} widthClass={widthClass} onToggle={toggle} />
    ))
    return (
        <View>
            <StepHeading
                title="Choose your apps"
                lead="These apps come with your server. Clear an app to hide it from everyone. You can show it again, or add more apps, at any time in Settings → Packages."
            />
            <View className="mb-6 flex-row flex-wrap gap-3">{cards}</View>
            <SetupContinueButton onPress={next} />
        </View>
    )
}
