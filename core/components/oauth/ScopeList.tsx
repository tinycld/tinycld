import { Text, View } from 'react-native'

// The consent screen must say what access means in plain language — a scope
// string tells a user nothing. The copy for each scope lives with the package
// that defines the scope (its oauth.RegisterPackage call), and the server
// returns it alongside the pending grant's scopes, so this component carries
// no copy of its own: a copy here would name packages core must not know, and
// would drift the day a package changed its wording.
//
// A scope the server sent no label for renders as its raw string. That is
// honest — hiding it would understate what is being granted.
interface ScopeListProps {
    scopes: string[]
    labels: Record<string, string>
}

export function ScopeList({ scopes, labels }: ScopeListProps) {
    if (scopes.length === 0) return null

    return (
        <View className="gap-2">
            {scopes.map(scope => (
                <View key={scope} className="flex-row items-start gap-2">
                    <Text className="text-foreground">•</Text>
                    <Text className="text-foreground flex-1">{labels[scope] ?? scope}</Text>
                </View>
            ))}
        </View>
    )
}
