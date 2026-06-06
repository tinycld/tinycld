import { Text, View } from 'react-native'

type StatusVariant =
    | 'bundled'
    | 'installed'
    | 'available'
    | 'disabled'
    | 'installing'
    | 'update-available'
    | 'current'
    | 'incompatible'
    | 'updating'
    | 'error'

const classNames: Record<StatusVariant, { bg: string; text: string }> = {
    bundled: { bg: 'bg-info', text: 'text-info-foreground' },
    installed: { bg: 'bg-success-soft', text: 'text-success-soft-foreground' },
    available: { bg: 'bg-muted', text: 'text-muted-foreground' },
    disabled: { bg: 'bg-danger-soft', text: 'text-danger-soft-foreground' },
    installing: { bg: 'bg-warning-soft', text: 'text-warning-soft-foreground' },
    'update-available': { bg: 'bg-warning-soft', text: 'text-warning-soft-foreground' },
    current: { bg: 'bg-success-soft', text: 'text-success-soft-foreground' },
    incompatible: { bg: 'bg-danger-soft', text: 'text-danger-soft-foreground' },
    updating: { bg: 'bg-info-soft', text: 'text-info-soft-foreground' },
    error: { bg: 'bg-danger-soft', text: 'text-danger-soft-foreground' },
}

// Friendly labels for the multi-word / non-obvious variants. Single-word legacy
// variants (bundled, installed, …) read fine as their key, so they fall through.
const labels: Partial<Record<StatusVariant, string>> = {
    'update-available': 'Update',
    updating: 'Updating',
    current: 'Current',
    incompatible: 'Incompatible',
    error: 'Unavailable',
}

export function PackageStatusBadge({ status }: { status: string }) {
    const key = status as StatusVariant
    const variant = classNames[key] ?? classNames.available
    const label = labels[key] ?? status

    return (
        <View className={`px-2 py-0.5 rounded-full ${variant.bg}`}>
            <Text className={`text-[11px] font-semibold ${variant.text}`}>{label}</Text>
        </View>
    )
}
