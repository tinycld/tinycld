import { type FormHost, useFormHosts } from '@tinycld/core/lib/forms/use-form-hosts'
import { slugOfGroup, useTermsOpen } from '@tinycld/core/lib/packages/use-package-terms'
import { usePackages } from '@tinycld/core/lib/packages/use-packages'
import { Pressable, Text, View } from 'react-native'

export interface FormsHostListProps {
    onPick: (hostKey: string) => void
}

interface HostRowProps {
    host: FormHost
    packageName: string
    onPick: (hostKey: string) => void
}

function HostRow({ host, packageName, onPick }: HostRowProps) {
    return (
        <Pressable
            onPress={() => onPick(host.key)}
            className="py-3 border-b border-border"
            accessibilityRole="button"
            testID={`forms-host-${host.key}`}
        >
            <Text className="text-foreground">{host.label}</Text>
            <Text className="text-xs text-muted-foreground">{packageName}</Text>
        </Pressable>
    )
}

export function FormsHostList({ onPick }: FormsHostListProps) {
    // A package whose terms the owner has not accepted offers no form host.
    const hosts = useTermsOpen(useFormHosts(), slugOfGroup)
    const packages = usePackages()
    const nameOf = (slug: string) => packages.find(p => p.slug === slug)?.name ?? slug
    const rows = hosts.map(h => (
        <HostRow key={h.key} host={h} packageName={nameOf(h.pkgSlug)} onPick={onPick} />
    ))
    return <View>{rows}</View>
}
