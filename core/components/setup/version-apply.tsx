import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Modal, ModalBackdrop, ModalContent } from '@tinycld/core/ui/modal'
import { AlertTriangle } from 'lucide-react-native'
import { useEffect, useState } from 'react'
import { Pressable, ScrollView, Text, TextInput, View } from 'react-native'
import type { DropReport, PendingChange } from './use-package-versions'

// ConfirmChangesModal gates Apply. For an upgrade-only set it is a simple
// confirm; when the set includes downgrades it loads a drop report for EVERY
// downgraded package and requires the operator to type each downgraded slug
// (comma/space separated) — so confirming one package can never silently
// downgrade another. An apply failure is surfaced inline (the modal stays open).
export function ConfirmChangesModal({
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
