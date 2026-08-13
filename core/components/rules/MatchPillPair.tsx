import { Pressable, Text, View } from 'react-native'

// Visual idiom copied from app/(app)/settings/audit-log.tsx's FilterChip —
// the closest existing in-repo precedent for a two-option toggle pill.
function MatchPill({
    label,
    isActive,
    onPress,
}: {
    label: string
    isActive: boolean
    onPress: () => void
}) {
    return (
        <Pressable
            onPress={onPress}
            className={`px-2.5 py-1 rounded-md ${isActive ? 'bg-primary' : 'border border-border'}`}
        >
            <Text className={`text-xs ${isActive ? 'text-primary-foreground' : 'text-primary'}`}>
                {label}
            </Text>
        </Pressable>
    )
}

export interface MatchPillPairProps {
    match: 'all' | 'any'
    onSelect: (match: 'all' | 'any') => void
}

export function MatchPillPair({ match, onSelect }: MatchPillPairProps) {
    return (
        <View className="flex-row gap-1.5">
            <MatchPill label="all" isActive={match === 'all'} onPress={() => onSelect('all')} />
            <MatchPill label="any" isActive={match === 'any'} onPress={() => onSelect('any')} />
        </View>
    )
}
