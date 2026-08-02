import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { PackageManager } from '@tinycld/core/components/setup/PackageManager'
import { getIcon } from '@tinycld/core/components/workspace/package-icon-map'
import { captureException } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { Divider } from '@tinycld/core/ui/divider'
import { Switch } from '@tinycld/core/ui/switch'
import { ArrowLeft } from 'lucide-react-native'
import { Pressable, ScrollView, Text, View } from 'react-native'

// Two tiers on one screen. Admins get per-org enablement — which packages this
// org's members see. Owners additionally get the install manager, because
// installing or removing a package rebuilds the artifact the whole deployment
// runs. requireOwner (server/coreserver/pkg_install.go) is the real
// enforcement; hiding the section just avoids offering a guaranteed 403.
export default function OrgPackageSettings() {
    const { isAdmin, isOwner, isReady } = useCurrentRole()
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const fgColor = useThemeColor('foreground')

    if (!isAdmin) {
        return (
            <View className="flex-1 p-5 items-center justify-center bg-background">
                <DocumentTitle pkg="Settings" title="Packages" />
                <Text className="text-muted-foreground text-base">
                    Only admins can manage packages.
                </Text>
            </View>
        )
    }

    return (
        <ScrollView className="flex-1 bg-background" contentContainerStyle={{ flexGrow: 1 }}>
            <DocumentTitle pkg="Settings" title="Packages" />
            <View className="p-5 w-full gap-4">
                <View className="flex-row gap-3 items-center">
                    <Pressable onPress={navigateBack}>
                        <ArrowLeft size={24} color={fgColor} />
                    </Pressable>
                    <Text className="text-foreground text-[22px] font-bold">Packages</Text>
                </View>
                <View className="max-w-[600px] w-full gap-4">
                    <Text className="text-muted-foreground text-sm">
                        Enable or disable packages for this organization. Disabled packages will be
                        hidden from all members.
                    </Text>
                    <PackageToggleList />
                </View>
                <InstallManagerSection isVisible={isReady && isOwner} />
            </View>
        </ScrollView>
    )
}

// Gated on isReady as well as isOwner: `role` reads null while the live query
// loads, so a bare isOwner check flash-hides the manager for a legitimate owner
// on a cold load. The wider column matches what the manager's rows need — the
// toggle list above keeps the standard 600px settings width.
function InstallManagerSection({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null

    return (
        <View className="w-full gap-4" style={{ maxWidth: 1040 }} testID="settings-install-manager">
            <Divider />
            <View className="gap-1">
                <Text className="text-foreground text-[17px] font-semibold">
                    Install & Versions
                </Text>
                <Text className="text-muted-foreground text-sm">
                    Install, remove, and change the version of packages for this deployment. These
                    changes rebuild the app and affect everyone.
                </Text>
            </View>
            <PackageManager pb={pb} isVisible />
        </View>
    )
}

function PackageToggleList() {
    const [pkgRegistryCollection] = useStore('pkg_registry')
    const [orgPkgEnabledCollection] = useStore('org_pkg_enabled')

    // Get all active packages from registry
    const { data: bundledPackages } = useLiveQuery(
        query =>
            query
                .from({ pkg_registry: pkgRegistryCollection })
                .where(({ pkg_registry }) => eq(pkg_registry.status, 'bundled')),
        []
    )

    const { data: installedPackages } = useLiveQuery(
        query =>
            query
                .from({ pkg_registry: pkgRegistryCollection })
                .where(({ pkg_registry }) => eq(pkg_registry.status, 'installed')),
        []
    )

    // Package enablement toggles (single-org: no org scoping)
    const { data: orgToggles } = useOrgLiveQuery(query =>
        query.from({ org_pkg_enabled: orgPkgEnabledCollection })
    )

    const allPackages = [...(bundledPackages ?? []), ...(installedPackages ?? [])].sort(
        (a, b) => (a.nav_order ?? 0) - (b.nav_order ?? 0)
    )

    if (allPackages.length === 0) {
        return (
            <View className="p-5 items-center rounded-xl border border-border bg-surface-secondary">
                <Text className="text-muted-foreground text-sm">No packages available.</Text>
            </View>
        )
    }

    const toggleMap = new Map(
        (orgToggles ?? []).map(t => [t.pkg, { id: t.id, enabled: t.enabled }])
    )

    return (
        <View className="rounded-xl border border-border overflow-hidden bg-surface-secondary">
            {allPackages.map((pkg, i) => (
                <View key={pkg.id}>
                    {i > 0 && <Divider />}
                    <PackageToggleRow pkg={pkg} toggle={toggleMap.get(pkg.slug)} />
                </View>
            ))}
        </View>
    )
}

function PackageToggleRow({
    pkg,
    toggle,
}: {
    pkg: { id: string; name: string; slug: string; icon: string; description: string }
    toggle?: { id: string; enabled: boolean }
}) {
    const fgColor = useThemeColor('foreground')
    const [orgPkgEnabledCollection] = useStore('org_pkg_enabled')

    const isEnabled = toggle ? toggle.enabled : true

    const upsertToggle = useMutation({
        mutationFn: mutation(function* ({ enabled }: { enabled: boolean }) {
            if (toggle) {
                yield orgPkgEnabledCollection.update(toggle.id, draft => {
                    draft.enabled = enabled
                })
            } else {
                yield orgPkgEnabledCollection.insert({
                    id: crypto.randomUUID().replace(/-/g, '').slice(0, 15),
                    pkg: pkg.slug,
                    enabled,
                    created: '',
                    updated: '',
                })
            }
        }),
        onError: err => captureException('Failed to toggle package', err),
    })

    const Icon = getIcon(pkg.icon)

    return (
        <View className="flex-row items-center justify-between px-4 py-3">
            <View className="flex-row items-center gap-3 flex-1">
                <Icon size={20} color={fgColor} />
                <View className="flex-1">
                    <Text className="text-foreground text-[15px] font-medium">{pkg.name}</Text>
                    {pkg.description && (
                        <Text className="text-muted-foreground text-xs">{pkg.description}</Text>
                    )}
                </View>
            </View>
            <Switch
                value={isEnabled}
                onValueChange={enabled => upsertToggle.mutate({ enabled })}
                disabled={upsertToggle.isPending}
            />
        </View>
    )
}
