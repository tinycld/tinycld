import { HelpIcon } from '@tinycld/core/components/help/HelpIcon'
import { getCoreConfigOptional } from '@tinycld/core/lib/core-config'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import {
    type CliDownload,
    downloadLabel,
    downloadUrl,
    useCliDownloads,
} from '@tinycld/core/lib/use-cli-downloads'
import {
    type ReleaseManifest,
    type ReleaseMember,
    useReleaseManifest,
} from '@tinycld/core/lib/use-release-manifest'
import AppUpdater from 'app-updater'
import { nativeBuildVersion } from 'expo-application'
import Constants from 'expo-constants'
import { Linking, Platform, Pressable, Text, View } from 'react-native'

const DEFAULT_PRIVACY_URL = 'https://tinycld.org/privacy'
const DEFAULT_SOURCE_URL = 'https://github.com/tinycld/tinycld'

// The running OTA bundle id (+ short hash), or null on web where there is no
// native updater. Read at render — the id is fixed for a running bundle's
// lifetime, so no state/effect is needed.
function bundleRowValue(): string | null {
    if (Platform.OS === 'web') return null
    const id = AppUpdater.getCurrentBundleId()
    const hash = AppUpdater.getCurrentBundleHash().slice(0, 8)
    return hash ? `${id} · ${hash}` : id
}

export function AboutSection() {
    const config = getCoreConfigOptional()
    const version = Constants.expoConfig?.version ?? 'unknown'
    const rawCommit = config?.release ?? 'dev'
    const commit = rawCommit.slice(0, 7)
    // Native build number (iOS CFBundleVersion / Android versionCode), null on
    // web and in some dev runs. Pairing it with the version + commit lets a tester
    // pin the EXACT binary a crash came from — two TestFlight builds share a
    // version but differ by build number (the "(48)" in an iOS crash report).
    const build = nativeBuildVersion
    const versionValue = build ? `${version} (${build}) · ${commit}` : `${version} (${commit})`
    // The OTA bundle actually executing right now: `embedded-<v>` on a fresh
    // binary, or `build-<ts>-<platform>` after an over-the-air update. The Version
    // row above is the NATIVE binary's version and never changes on OTA, so this is
    // the only field that tells you which JS bundle you're on. Paired with a short
    // hash to disambiguate two builds. Omitted on web (no native updater).
    const bundleValue = bundleRowValue()
    const server = getResolvedAddress() ?? 'not connected'
    const privacyUrl = config?.privacyUrl ?? DEFAULT_PRIVACY_URL
    const sourceUrl = config?.sourceUrl ?? DEFAULT_SOURCE_URL

    const { data: manifest } = useReleaseManifest()

    return (
        <View className="gap-3">
            <Text className="text-xl font-bold text-foreground">About</Text>
            <View className="rounded-xl border border-border bg-surface-secondary p-4 gap-2">
                <Row label="Version" value={versionValue} />
                {bundleValue ? <Row label="Bundle" value={bundleValue} /> : null}
                <Row label="Server" value={server} />
                <Pressable onPress={() => Linking.openURL(privacyUrl)}>
                    <Text className="text-sm text-primary">Privacy policy</Text>
                </Pressable>
                <Pressable onPress={() => Linking.openURL(sourceUrl)}>
                    <Text className="text-sm text-primary">Source code (AGPL-3.0)</Text>
                </Pressable>
            </View>
            <IncludedPackages manifest={manifest} />
            <CommandLineTools />
        </View>
    )
}

// Offers the per-org CLI binaries the build pipeline cross-compiled, the
// viewer's own OS first. Returns null when none are built (dev server, fresh
// image before the first rebuild) — the CLI build is best-effort, so an empty
// list is a normal state, not an error.
function CommandLineTools() {
    const { downloads, detectedOS } = useCliDownloads()
    const host = (getResolvedAddress() ?? 'your-server').replace(/^https?:\/\//, '')
    if (downloads.length === 0) return null

    const note =
        'After downloading, run `chmod +x tinycld`. Binaries are unsigned in this version: ' +
        'on macOS run `xattr -d com.apple.quarantine ./tinycld`; on Windows choose ' +
        `More info → Run anyway. Then \`tinycld auth login ${host}\` to get started.`

    return (
        <View className="gap-2">
            <View className="flex-row items-center gap-1.5">
                <Text className="text-base font-semibold text-foreground">Command line tools</Text>
                <HelpIcon topic="core:command-line" />
            </View>
            <View className="rounded-xl border border-border bg-surface-secondary p-4 gap-2">
                {downloads.map(d => (
                    <DownloadRow key={d.platform} download={d} isDetected={d.os === detectedOS} />
                ))}
                <Text className="text-xs text-muted-foreground">{note}</Text>
            </View>
        </View>
    )
}

function DownloadRow({ download, isDetected }: { download: CliDownload; isDetected: boolean }) {
    const label = isDetected
        ? `${downloadLabel(download)} · this computer`
        : downloadLabel(download)
    return (
        <View className="flex-row justify-between items-center">
            <Text className="text-sm text-muted-foreground">{label}</Text>
            <Pressable onPress={() => Linking.openURL(downloadUrl(download))}>
                <Text className="text-sm text-primary">Download</Text>
            </Pressable>
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
