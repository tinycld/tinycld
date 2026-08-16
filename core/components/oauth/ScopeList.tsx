import { Text, View } from 'react-native'

// Human-readable copy for each scope. The consent screen must say what access
// means in plain language — "mail:read" tells a user nothing.
//
// EVERY scope in oauth.AllScopes needs an entry. A missing one is not a
// cosmetic gap: the fallback below renders the raw scope string, so the
// consent screen asks a person to approve "cards:write". scope-labels.test.ts
// reads the Go constant and fails when the two drift, which is how the cards
// scopes were found to have been unlabeled since they shipped.
export const SCOPE_LABELS: Record<string, string> = {
    profile: 'See your name and email address',
    'mail:read': 'Read your email',
    'mail:send': 'Send email on your behalf',
    'drive:read': 'Read your files',
    'drive:write': 'Create and modify your files',
    'contacts:read': 'Read your contacts',
    'contacts:write': 'Create and modify your contacts',
    'calendar:read': 'Read your calendar',
    'calendar:write': 'Create and modify calendar events',
    'cards:read': 'Read your boards and cards',
    'cards:write': 'Create and modify your boards and cards',
    'text:read': 'Read comments on your documents',
    'text:write': 'Add and resolve comments on your documents',
    'calc:read': 'Read comments on your spreadsheets',
    'calc:write': 'Add and resolve comments on your spreadsheets',
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
