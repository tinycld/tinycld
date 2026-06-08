import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import type { LucideIcon } from 'lucide-react-native'
import type { ReactNode } from 'react'
import { Text, View } from 'react-native'

// Shared presentational primitives for the setup console. Behaviour-free — they
// only standardize the layout/typography idioms repeated across every tab so the
// tabs themselves stay focused on data + actions.

// A page header: large title, optional descriptive subtitle, and a slot on the
// right for primary/secondary actions. Keeps each tab's top section identical.
export function PageHeader({
    title,
    subtitle,
    actions,
}: {
    title: string
    subtitle?: string
    actions?: ReactNode
}) {
    return (
        <View className="flex-row items-start justify-between gap-6">
            <View className="flex-1">
                <Text className="text-foreground" style={{ fontSize: 28, fontWeight: '700' }}>
                    {title}
                </Text>
                {subtitle ? (
                    <Text
                        className="text-muted-foreground"
                        style={{ fontSize: 15, marginTop: 6, maxWidth: 620 }}
                    >
                        {subtitle}
                    </Text>
                ) : null}
            </View>
            {actions ? <View className="flex-row gap-2.5 items-center">{actions}</View> : null}
        </View>
    )
}

// An uppercase, tracked-out label used above a fieldset or a list section.
export function SectionLabel({ children }: { children: ReactNode }) {
    return (
        <Text
            className="text-muted-foreground"
            style={{
                fontSize: 11,
                fontWeight: '600',
                textTransform: 'uppercase',
                letterSpacing: 1,
            }}
        >
            {children}
        </Text>
    )
}

// A monospace pill for slugs / package names / versions — the console's signal
// that a value is an identifier rather than prose.
export function SlugTag({ children }: { children: ReactNode }) {
    return (
        <View className="px-2 py-0.5 rounded-md bg-surface-secondary border border-border">
            <Text
                className="text-muted-foreground"
                style={{ fontFamily: 'monospace', fontSize: 11.5 }}
            >
                {children}
            </Text>
        </View>
    )
}

// A rounded square holding a feature's lucide icon, used as the leading element
// of a list row.
export function RowIcon({ Icon, accent }: { Icon: LucideIcon; accent?: boolean }) {
    const mutedColor = useThemeColor('muted-foreground')
    const primaryColor = useThemeColor('primary')
    return (
        <View className="w-10 h-10 rounded-xl items-center justify-center bg-surface-secondary border border-border">
            <Icon size={19} color={accent ? primaryColor : mutedColor} />
        </View>
    )
}
