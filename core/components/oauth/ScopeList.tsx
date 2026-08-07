import { Text, View } from 'react-native'

// Human-readable copy for each scope. The consent screen must say what access
// means in plain language — "mail:read" tells a user nothing.
const SCOPE_LABELS: Record<string, string> = {
    profile: 'See your name and email address',
    'mail:read': 'Read your email',
    'mail:send': 'Send email on your behalf',
    'drive:read': 'Read your files',
    'drive:write': 'Create and modify your files',
    'contacts:read': 'Read your contacts',
    'contacts:write': 'Create and modify your contacts',
    'calendar:read': 'Read your calendar',
    'calendar:write': 'Create and modify calendar events',
}

interface ScopeListProps {
    scopes: string[]
}

export function ScopeList({ scopes }: ScopeListProps) {
    if (scopes.length === 0) return null

    return (
        <View className="gap-2">
            {scopes.map(scope => (
                <View key={scope} className="flex-row items-start gap-2">
                    <Text className="text-foreground">•</Text>
                    <Text className="text-foreground flex-1">{SCOPE_LABELS[scope] ?? scope}</Text>
                </View>
            ))}
        </View>
    )
}
