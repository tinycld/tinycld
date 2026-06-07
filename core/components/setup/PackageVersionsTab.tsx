import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Divider } from '@tinycld/core/ui/divider'
import { Menu } from '@tinycld/core/ui/menu'
import { Modal, ModalBackdrop, ModalContent } from '@tinycld/core/ui/modal'
import { AlertTriangle } from 'lucide-react-native'
import type PocketBase from 'pocketbase'
import { useEffect, useState } from 'react'
import { ActivityIndicator, Pressable, ScrollView, Text, TextInput, View } from 'react-native'
import { InstallProgressModal } from './InstallProgressModal'
import { PackageStatusBadge } from './PackageStatusBadge'
import {
    type CompatViolation,
    type DropReport,
    type PackageVersionInfo,
    type PendingChange,
    usePackageVersions,
} from './use-package-versions'

interface PackageVersionsTabProps {
    isVisible: boolean
    pb: PocketBase
}

export function PackageVersionsTab({ isVisible, pb }: PackageVersionsTabProps) {
    const vm = usePackageVersions(pb)
    const [confirmOpen, setConfirmOpen] = useState(false)

    if (!isVisible) return null

    const onApplyPressed = () => setConfirmOpen(true)

    return (
        <>
            <View className="flex-row justify-between items-center">
                <Text className="text-foreground" style={{ fontSize: 24, fontWeight: 'bold' }}>
                    Versions
                </Text>
                <SelectionActions
                    pendingCount={vm.pendingChanges.length}
                    onSelectAll={vm.selectAllUpdates}
                    onClear={vm.clearSelection}
                />
            </View>

            <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                Update or downgrade installed packages. Downgrades that drop data require
                confirmation. Selected packages are checked for compatibility before applying.
            </Text>

            <ViolationList violations={vm.violations} isChecking={vm.isChecking} />

            <VersionList
                versions={vm.versions}
                names={vm.names}
                isLoading={vm.isLoading}
                targets={vm.targets}
                onSetTarget={vm.setTarget}
            />

            <ApplyBar
                pendingChanges={vm.pendingChanges}
                canApply={vm.canApply}
                onApply={onApplyPressed}
            />

            <ConfirmChangesModal
                isOpen={confirmOpen}
                pendingChanges={vm.pendingChanges}
                hasDowngrade={vm.hasDowngrade}
                fetchDropReport={vm.fetchDropReport}
                onCancel={() => setConfirmOpen(false)}
                onConfirm={async () => {
                    // Apply first; only close on success. If it throws, the modal
                    // catches it and shows the error inline (don't pre-close).
                    await vm.applyChanges()
                    setConfirmOpen(false)
                }}
            />

            <InstallProgressModal
                isVisible={vm.applyJobId !== null}
                jobId={vm.applyJobId}
                authToken={pb.authStore.token}
                onClose={vm.onApplyComplete}
                onComplete={vm.refresh}
            />
        </>
    )
}

function SelectionActions({
    pendingCount,
    onSelectAll,
    onClear,
}: {
    pendingCount: number
    onSelectAll: () => void
    onClear: () => void
}) {
    return (
        <View className="flex-row gap-2 items-center">
            {pendingCount > 0 && (
                <Pressable onPress={onClear} className="px-3 py-2">
                    <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                        Clear ({pendingCount})
                    </Text>
                </Pressable>
            )}
            <Pressable
                onPress={onSelectAll}
                className="px-3 py-2 rounded-lg border border-muted-foreground/25"
            >
                <Text className="text-muted-foreground" style={{ fontWeight: '500', fontSize: 13 }}>
                    Select all updates
                </Text>
            </Pressable>
        </View>
    )
}

function ViolationList({
    violations,
    isChecking,
}: {
    violations: CompatViolation[]
    isChecking: boolean
}) {
    if (isChecking) {
        return (
            <Text className="text-muted-foreground" style={{ fontSize: 12 }}>
                Checking compatibility…
            </Text>
        )
    }
    if (violations.length === 0) return null
    return (
        <View className="rounded-lg p-3 gap-1 bg-danger-soft">
            <Text className="text-danger" style={{ fontSize: 13, fontWeight: '600' }}>
                Incompatible selection
            </Text>
            {violations.map(v => (
                <Text
                    key={`${v.package}:${v.requires}`}
                    className="text-danger"
                    style={{ fontSize: 12 }}
                >
                    {v.package} requires {v.requires} {v.range} — found {v.found || 'not installed'}
                </Text>
            ))}
        </View>
    )
}

function VersionList({
    versions,
    names,
    isLoading,
    targets,
    onSetTarget,
}: {
    versions: PackageVersionInfo[]
    names: Record<string, string>
    isLoading: boolean
    targets: Record<string, string>
    onSetTarget: (slug: string, version: string) => void
}) {
    const mutedColor = useThemeColor('muted-foreground')

    if (isLoading) {
        return (
            <View className="p-5 items-center">
                <ActivityIndicator size="large" color={mutedColor} />
            </View>
        )
    }
    if (versions.length === 0) {
        return (
            <View className="p-5 items-center rounded-xl border bg-surface-secondary border-border">
                <Text className="text-muted-foreground" style={{ fontSize: 15 }}>
                    No packages installed.
                </Text>
            </View>
        )
    }
    return (
        <View className="rounded-xl border overflow-hidden bg-surface-secondary border-border">
            {versions.map((info, i) => (
                <View key={info.slug}>
                    {i > 0 && <Divider />}
                    <VersionRow
                        info={info}
                        name={names[info.slug] ?? info.slug}
                        target={targets[info.slug]}
                        onSetTarget={onSetTarget}
                    />
                </View>
            ))}
        </View>
    )
}

function rowStatus(info: PackageVersionInfo, target?: string): string {
    if (target) return 'updating'
    if (info.error) return 'error'
    if (info.hasUpdate) return 'update-available'
    return 'current'
}

function VersionRow({
    info,
    name,
    target,
    onSetTarget,
}: {
    info: PackageVersionInfo
    name: string
    target?: string
    onSetTarget: (slug: string, version: string) => void
}) {
    const options = buildVersionOptions(info)
    const status = rowStatus(info, target)

    return (
        <View className="flex-row items-center px-4 py-3.5 gap-3">
            <View className="flex-1 gap-0.5">
                <View className="flex-row gap-2 items-center">
                    <Text className="text-foreground" style={{ fontSize: 15, fontWeight: '600' }}>
                        {name}
                    </Text>
                    <PackageStatusBadge status={status} />
                </View>
                <Text className="text-muted-foreground" style={{ fontSize: 12 }}>
                    {info.slug} · current v{info.current || '—'}
                    {info.source !== 'unknown' ? ` · ${info.source}` : ''}
                </Text>
                {info.error ? (
                    <Text className="text-danger" style={{ fontSize: 11 }} numberOfLines={1}>
                        Couldn't check versions
                    </Text>
                ) : null}
            </View>
            <View style={{ width: 160 }}>
                <RowVersionSelect
                    options={options}
                    value={target ?? info.current}
                    disabled={options.length <= 1}
                    onChange={v => onSetTarget(info.slug, v)}
                />
            </View>
        </View>
    )
}

function buildVersionOptions(info: PackageVersionInfo) {
    const list = info.available.length > 0 ? info.available : info.current ? [info.current] : []
    return list.map(v => ({
        label: v === info.current ? `v${v} (current)` : `v${v}`,
        value: v,
    }))
}

// RowVersionSelect is a standalone (non-RHF) version picker. SelectInput is
// react-hook-form-bound, so for a per-row select we use the anchored Menu
// primitive (correct positioning + a11y) rather than a form or a center-screen
// modal. A disabled row (≤1 option) renders plain text.
function RowVersionSelect({
    options,
    value,
    disabled,
    onChange,
}: {
    options: { label: string; value: string }[]
    value: string
    disabled: boolean
    onChange: (v: string) => void
}) {
    const fgColor = useThemeColor('foreground')
    const selected = options.find(o => o.value === value)
    const label = selected?.label ?? `v${value}`

    if (disabled) {
        return (
            <Text className="text-muted-foreground" style={{ fontSize: 13, textAlign: 'right' }}>
                {label}
            </Text>
        )
    }

    return (
        <Menu>
            <Menu.Trigger>
                <Pressable className="px-3 py-2 rounded-lg border border-muted-foreground/25">
                    <Text style={{ color: fgColor, fontSize: 13 }} numberOfLines={1}>
                        {label}
                    </Text>
                </Pressable>
            </Menu.Trigger>
            <Menu.Portal>
                <Menu.Overlay />
                <Menu.Content align="end" className="max-h-[360px]">
                    <ScrollView>
                        {options.map(o => (
                            <Menu.Item
                                key={o.value}
                                onPress={() => onChange(o.value)}
                                className={o.value === value ? 'bg-accent/20' : ''}
                            >
                                <Menu.ItemTitle>{o.label}</Menu.ItemTitle>
                            </Menu.Item>
                        ))}
                    </ScrollView>
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}

function ApplyBar({
    pendingChanges,
    canApply,
    onApply,
}: {
    pendingChanges: PendingChange[]
    canApply: boolean
    onApply: () => void
}) {
    if (pendingChanges.length === 0) return null
    return (
        <Pressable
            onPress={onApply}
            disabled={!canApply}
            className={`px-4 py-3 rounded-lg self-start bg-primary ${canApply ? 'opacity-100' : 'opacity-50'}`}
        >
            <Text className="text-primary-foreground" style={{ fontWeight: '600', fontSize: 14 }}>
                Apply {pendingChanges.length} change{pendingChanges.length === 1 ? '' : 's'}
            </Text>
        </Pressable>
    )
}

// ConfirmChangesModal gates Apply. For an upgrade-only set it is a simple
// confirm; when the set includes downgrades it loads a drop report for EVERY
// downgraded package and requires the operator to type each downgraded slug
// (comma/space separated) — so confirming one package can never silently
// downgrade another. An apply failure is surfaced inline (the modal stays open).
function ConfirmChangesModal({
    isOpen,
    pendingChanges,
    hasDowngrade,
    fetchDropReport,
    onCancel,
    onConfirm,
}: {
    isOpen: boolean
    pendingChanges: PendingChange[]
    hasDowngrade: boolean
    fetchDropReport: (slug: string, targetVersion: string) => Promise<DropReport>
    onCancel: () => void
    onConfirm: () => Promise<void>
}) {
    const dangerColor = useThemeColor('danger')
    const fgColor = useThemeColor('foreground')
    const [typed, setTyped] = useState('')
    const [reports, setReports] = useState<Record<string, DropReport>>({})
    const [loadingReports, setLoadingReports] = useState(false)
    const [submitting, setSubmitting] = useState(false)
    const [submitError, setSubmitError] = useState<string | null>(null)

    const downgrades = pendingChanges.filter(c => c.isDowngrade)
    const downgradeSlugs = downgrades.map(c => c.slug)
    // Stable string key of the downgraded {slug→version} set — used as the effect
    // dep so it re-runs only when the set (or a target version) actually changes,
    // not on every render. Pairs are re-parsed from it inside the effect.
    const downgradeKey = downgrades.map(c => `${c.slug}@${c.targetVersion}`).join(',')

    // Load a drop report for EVERY downgraded package when the confirm opens.
    // Reset reports at the top so a previous open's data never flashes through.
    useEffect(() => {
        const pairs = downgradeKey ? downgradeKey.split(',').map(p => p.split('@')) : []
        if (!isOpen || pairs.length === 0) return
        let cancelled = false
        setReports({})
        setLoadingReports(true)
        Promise.all(
            pairs.map(([slug, version]) =>
                fetchDropReport(slug, version)
                    .then(r => [slug, r] as const)
                    .catch(() => [slug, { droppedCollections: [], droppedFields: [] }] as const)
            )
        )
            .then(entries => !cancelled && setReports(Object.fromEntries(entries)))
            .finally(() => !cancelled && setLoadingReports(false))
        return () => {
            cancelled = true
        }
    }, [isOpen, downgradeKey, fetchDropReport])

    if (!isOpen) return null

    const typedSlugs = new Set(
        typed
            .split(/[,\s]+/)
            .map(s => s.trim())
            .filter(Boolean)
    )
    const allConfirmed = downgradeSlugs.every(s => typedSlugs.has(s))
    const confirmEnabled = (hasDowngrade ? allConfirmed : true) && !submitting

    const handleConfirm = async () => {
        setSubmitting(true)
        setSubmitError(null)
        try {
            await onConfirm()
            setTyped('')
            setReports({})
        } catch (err) {
            // Surface the failure inline instead of leaving the user with a
            // silently-closed modal and no feedback.
            setSubmitError(err instanceof Error ? err.message : 'Failed to apply changes')
        } finally {
            setSubmitting(false)
        }
    }

    const handleCancel = () => {
        setTyped('')
        setReports({})
        setSubmitError(null)
        onCancel()
    }

    const confirmHint =
        downgradeSlugs.length === 1
            ? `Type ${downgradeSlugs[0]} to confirm`
            : `Type each package name to confirm: ${downgradeSlugs.join(', ')}`

    return (
        <Modal isOpen onClose={handleCancel}>
            <ModalBackdrop />
            <ModalContent className="w-[420px] p-4 gap-3 max-h-[560px]">
                <View className="flex-row gap-2 items-center">
                    {hasDowngrade && <AlertTriangle size={18} color={dangerColor} />}
                    <Text className="text-foreground" style={{ fontSize: 19, fontWeight: '600' }}>
                        {hasDowngrade ? 'Confirm downgrade' : 'Apply version changes'}
                    </Text>
                </View>

                <ScrollView className="max-h-[360px]">
                    <ChangeSummary pendingChanges={pendingChanges} />
                    <DropReportList
                        isVisible={hasDowngrade}
                        loading={loadingReports}
                        slugs={downgradeSlugs}
                        reports={reports}
                    />
                </ScrollView>

                {hasDowngrade && (
                    <View className="gap-1">
                        <Text className="text-muted-foreground" style={{ fontSize: 12 }}>
                            {confirmHint}
                        </Text>
                        <TextInput
                            value={typed}
                            onChangeText={setTyped}
                            autoCapitalize="none"
                            placeholder={downgradeSlugs.join(', ')}
                            className="rounded-lg border border-border px-3 py-2"
                            style={{ color: fgColor, fontSize: 14 }}
                        />
                    </View>
                )}

                {submitError && (
                    <View className="rounded-lg p-2 bg-danger-soft">
                        <Text className="text-danger" style={{ fontSize: 12 }}>
                            {submitError}
                        </Text>
                    </View>
                )}

                <View className="flex-row gap-3 justify-end">
                    <Pressable onPress={handleCancel} className="px-3 py-2" disabled={submitting}>
                        <Text className="text-foreground" style={{ fontSize: 13 }}>
                            Cancel
                        </Text>
                    </Pressable>
                    <Pressable
                        onPress={handleConfirm}
                        disabled={!confirmEnabled}
                        className={`px-4 py-2 rounded-lg ${hasDowngrade ? 'bg-danger' : 'bg-primary'} ${confirmEnabled ? 'opacity-100' : 'opacity-50'}`}
                    >
                        <Text
                            className={
                                hasDowngrade ? 'text-danger-foreground' : 'text-primary-foreground'
                            }
                            style={{ fontSize: 13, fontWeight: '600' }}
                        >
                            {submitting ? 'Applying…' : hasDowngrade ? 'Downgrade' : 'Apply'}
                        </Text>
                    </Pressable>
                </View>
            </ModalContent>
        </Modal>
    )
}

function ChangeSummary({ pendingChanges }: { pendingChanges: PendingChange[] }) {
    return (
        <View className="gap-1 mb-2">
            {pendingChanges.map(c => (
                <Text key={c.slug} className="text-foreground" style={{ fontSize: 13 }}>
                    {c.slug} → v{c.targetVersion}
                    {c.isDowngrade ? ' (downgrade)' : ''}
                </Text>
            ))}
        </View>
    )
}

// DropReportList renders a drop report per downgraded package, so a multi-package
// downgrade shows every package's data loss — not just the first.
function DropReportList({
    isVisible,
    loading,
    slugs,
    reports,
}: {
    isVisible: boolean
    loading: boolean
    slugs: string[]
    reports: Record<string, DropReport>
}) {
    if (!isVisible) return null
    if (loading) {
        return (
            <Text className="text-muted-foreground" style={{ fontSize: 12 }}>
                Checking what data will be dropped…
            </Text>
        )
    }
    return (
        <View className="gap-2">
            {slugs.map(slug => (
                <DropReportCard key={slug} slug={slug} report={reports[slug]} />
            ))}
            <Text className="text-muted-foreground" style={{ fontSize: 11 }}>
                The database is backed up before the change.
            </Text>
        </View>
    )
}

function DropReportCard({ slug, report }: { slug: string; report?: DropReport }) {
    if (!report) return null
    const nothing = report.droppedCollections.length === 0 && report.droppedFields.length === 0
    if (nothing) {
        return (
            <Text className="text-muted-foreground" style={{ fontSize: 12 }}>
                {slug}: no collections or fields will be dropped.
            </Text>
        )
    }
    return (
        <View className="rounded-lg p-3 gap-1 bg-danger-soft">
            <Text className="text-danger" style={{ fontSize: 12, fontWeight: '600' }}>
                Downgrading {slug} will drop:
            </Text>
            {report.droppedCollections.map(c => (
                <Text key={c} className="text-danger" style={{ fontSize: 12 }}>
                    • collection {c}
                </Text>
            ))}
            {report.droppedFields.map(f => (
                <Text
                    key={`${f.collection}.${f.field}`}
                    className="text-danger"
                    style={{ fontSize: 12 }}
                >
                    • {f.collection}.{f.field}
                </Text>
            ))}
        </View>
    )
}
