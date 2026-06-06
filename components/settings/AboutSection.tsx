import { getCoreConfigOptional } from '@tinycld/core/lib/core-config'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import {
    type ReleaseManifest,
    type ReleaseMember,
    useReleaseManifest,
} from '@tinycld/core/lib/use-release-manifest'
import Constants from 'expo-constants'
import { Linking, Pressable, Text, View } from 'react-native'

const DEFAULT_PRIVACY_URL = 'https://tinycld.org/privacy'
const DEFAULT_SOURCE_URL = 'https://github.com/tinycld/tinycld'

export function AboutSection() {
    const config = getCoreConfigOptional()
    const version = Constants.expoConfig?.version ?? 'unknown'
    const rawCommit = config?.release ?? 'dev'
    const commit = rawCommit.slice(0, 7)
    const server = getResolvedAddress() ?? 'not connected'
    const privacyUrl = config?.privacyUrl ?? DEFAULT_PRIVACY_URL
    const sourceUrl = config?.sourceUrl ?? DEFAULT_SOURCE_URL

    const { data: manifest } = useReleaseManifest()

    return (
        <View className="gap-3">
            <Text className="text-xl font-bold text-foreground">About</Text>
            <View className="rounded-xl border border-border bg-surface-secondary p-4 gap-2">
                <Row label="Version" value={`${version} (${commit})`} />
                <Row label="Server" value={server} />
                <Pressable onPress={() => Linking.openURL(privacyUrl)}>
                    <Text className="text-sm text-primary">Privacy policy</Text>
                </Pressable>
                <Pressable onPress={() => Linking.openURL(sourceUrl)}>
                    <Text className="text-sm text-primary">Source code (AGPL-3.0)</Text>
                </Pressable>
            </View>
            <IncludedPackages manifest={manifest} />
        </View>
    )
}

// Derive the displayed version from a v* tag (the tag is the canonical
// released version) and pair it with a short SHA for the commit column.
function memberRows(members: ReleaseMember[]) {
    return members.map(m => ({
        name: m.name,
        version: m.tag.replace(/^v/, ''),
        commit: m.sha.slice(0, 7),
    }))
}

function formatReleasedAt(iso: string | undefined): string | null {
    if (!iso) return null
    const date = new Date(iso)
    if (Number.isNaN(date.getTime())) return null
    return date.toLocaleDateString()
}

// Renders the per-package version inventory baked into the running image.
// Returns null on non-release builds (empty members) so local/dev installs
// show nothing extra; the data comes from the /api/release endpoint.
function IncludedPackages({ manifest }: { manifest: ReleaseManifest | undefined }) {
    const members = manifest?.members ?? []
    if (members.length === 0) return null

    const rows = memberRows(members)
    const releasedAt = formatReleasedAt(manifest?.releasedAt)

    return (
        <View className="gap-2">
            <Text className="text-base font-semibold text-foreground">Included packages</Text>
            <View className="rounded-xl border border-border bg-surface-secondary p-4 gap-2">
                {rows.map(row => (
                    <Row key={row.name} label={row.name} value={`${row.version} (${row.commit})`} />
                ))}
                {releasedAt ? <Row label="Released" value={releasedAt} /> : null}
            </View>
        </View>
    )
}

function Row({ label, value }: { label: string; value: string }) {
    return (
        <View className="flex-row justify-between items-center">
            <Text className="text-sm text-muted-foreground">{label}</Text>
            <Text className="text-sm text-foreground" numberOfLines={1}>
                {value}
            </Text>
        </View>
    )
}
