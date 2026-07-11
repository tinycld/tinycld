import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Clock } from 'lucide-react-native'
import { Text, View } from 'react-native'
import { type OrgRole, ROLE_LABELS, ROLE_SWATCH } from './types'

export function RoleBadge({ role, size = 'md' }: { role: OrgRole; size?: 'sm' | 'md' }) {
    const swatch = ROLE_SWATCH[role]
    const py = size === 'sm' ? 2 : 3
    const px = size === 'sm' ? 7 : 9
    const fontSize = size === 'sm' ? 10.5 : 11.5
    return (
        <View
            className={`rounded-full border ${swatch.bg} ${swatch.border}`}
            style={{ paddingVertical: py, paddingHorizontal: px }}
        >
            <Text
                className={swatch.text}
                style={{
                    fontSize,
                    fontWeight: '700',
                    letterSpacing: 0.3,
                    textTransform: 'uppercase',
                }}
            >
                {ROLE_LABELS[role]}
            </Text>
        </View>
    )
}

export function PendingBadge() {
    const warningColor = useThemeColor('warning')
    return (
        <View
            className="flex-row items-center gap-1 rounded-full border bg-warning/10 border-warning/40"
            style={{ paddingVertical: 3, paddingHorizontal: 8 }}
        >
            <Clock size={10} color={warningColor} />
            <Text
                className="text-warning"
                style={{
                    fontSize: 10.5,
                    fontWeight: '700',
                    letterSpacing: 0.3,
                    textTransform: 'uppercase',
                }}
            >
                Pending
            </Text>
        </View>
    )
}

export function YouBadge() {
    const primary = useThemeColor('primary')
    return (
        <View
            style={{
                paddingVertical: 1,
                paddingHorizontal: 6,
                borderRadius: 999,
                backgroundColor: `${primary}1F`,
            }}
        >
            <Text style={{ fontSize: 10, fontWeight: '700', color: primary, letterSpacing: 0.3 }}>
                YOU
            </Text>
        </View>
    )
}
