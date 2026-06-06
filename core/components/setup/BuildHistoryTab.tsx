import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { captureException } from '@tinycld/core/lib/errors'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Divider } from '@tinycld/core/ui/divider'
import { History, RotateCcw, Trash2, X } from 'lucide-react-native'
import type PocketBase from 'pocketbase'
import { useCallback, useEffect, useState } from 'react'
import { ActivityIndicator, Pressable, Text, View } from 'react-native'
import { InstallProgressModal } from './InstallProgressModal'

interface BuildRecord {
    id: string
    build_id: string
    pkg_slug: string
    npm_package: string
    version: string
    action: string
    binary_archived: boolean
    release_id: string
    migrations_applied: number
    status: string
    created: string
}

function formatWhen(iso: string) {
    if (!iso) return ''
    return new Date(iso).toLocaleString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
    })
}

export function BuildHistoryTab({ isVisible, pb }: { isVisible: boolean; pb: PocketBase }) {
    const mutedColor = useThemeColor('muted-foreground')
    const [builds, setBuilds] = useState<BuildRecord[]>([])
    const [isLoading, setIsLoading] = useState(true)
    const [jobId, setJobId] = useState<string | null>(null)

    const fetchBuilds = useCallback(async () => {
        setIsLoading(true)
        try {
            const records = await pb
                .collection('pkg_build')
                .getFullList<BuildRecord>({ sort: '-created' })
            setBuilds(records)
        } catch (err) {
            captureException('setup.buildHistory.fetch', err)
        } finally {
            setIsLoading(false)
        }
    }, [pb])

    useEffect(() => {
        if (isVisible) fetchBuilds()
    }, [isVisible, fetchBuilds])

    if (!isVisible) return null

    // The builds newer than a given one are the ones a revert to it will
    // invalidate — surfaced in the confirm dialog so the operator sees the cost.
    const newerThan = (build: BuildRecord) =>
        builds.filter(b => b.created > build.created && b.action === 'install')

    return (
        <View className="gap-5">
            <Text className="text-foreground" style={{ fontSize: 24, fontWeight: 'bold' }}>
                Build History
            </Text>
            <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                Each successful package install is saved as a restorable build. Reverting to an
                older build reverses every schema change made since it (your data is preserved) and
                permanently invalidates the newer builds. Archives are kept until you delete them.
            </Text>

            <InstallProgressModal
                isVisible={jobId !== null}
                jobId={jobId}
                authToken={pb.authStore.token}
                onClose={() => {
                    setJobId(null)
                    fetchBuilds()
                }}
                onComplete={fetchBuilds}
            />

            <BuildList
                builds={builds}
                isLoading={isLoading}
                mutedColor={mutedColor}
                pb={pb}
                newerThan={newerThan}
                onJobStarted={setJobId}
                onChanged={fetchBuilds}
            />
        </View>
    )
}

function BuildList({
    builds,
    isLoading,
    mutedColor,
    pb,
    newerThan,
    onJobStarted,
    onChanged,
}: {
    builds: BuildRecord[]
    isLoading: boolean
    mutedColor: string
    pb: PocketBase
    newerThan: (b: BuildRecord) => BuildRecord[]
    onJobStarted: (jobId: string) => void
    onChanged: () => void
}) {
    if (isLoading) {
        return (
            <View className="p-8 items-center">
                <ActivityIndicator size="large" color={mutedColor} />
            </View>
        )
    }

    if (builds.length === 0) {
        return (
            <View className="p-8 items-center rounded-xl border gap-2 bg-surface-secondary border-border">
                <History size={32} color={`${mutedColor}60`} />
                <Text className="text-muted-foreground" style={{ fontSize: 15 }}>
                    No builds yet. Installing a package saves its first build.
                </Text>
            </View>
        )
    }

    return (
        <View className="rounded-xl border overflow-hidden bg-surface-secondary border-border">
            {builds.map((build, i) => (
                <View key={build.id}>
                    {i > 0 && <Divider />}
                    <BuildRow
                        build={build}
                        invalidates={newerThan(build)}
                        pb={pb}
                        onJobStarted={onJobStarted}
                        onChanged={onChanged}
                    />
                </View>
            ))}
        </View>
    )
}

function BuildRow({
    build,
    invalidates,
    pb,
    onJobStarted,
    onChanged,
}: {
    build: BuildRecord
    invalidates: BuildRecord[]
    pb: PocketBase
    onJobStarted: (jobId: string) => void
    onChanged: () => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const [confirmRevert, setConfirmRevert] = useState(false)
    const [confirmDelete, setConfirmDelete] = useState(false)
    const [error, setError] = useState<string | null>(null)

    const isCurrent = build.status === 'current'
    const isSuperseded = build.status === 'superseded'
    const isRevertMarker = build.action === 'revert'

    const doRevert = async () => {
        setError(null)
        try {
            const res = await fetch(`${PB_SERVER_ADDR}/api/admin/packages/revert`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    Authorization: pb.authStore.token,
                },
                body: JSON.stringify({ buildId: build.build_id }),
            })
            const data = await res.json()
            if (!res.ok) {
                setError(data.error ?? 'Failed to start revert')
                return
            }
            setConfirmRevert(false)
            onJobStarted(data.jobId)
        } catch (err) {
            captureException('setup.buildHistory.revert', err)
            setError('Failed to start revert')
        }
    }

    const doDelete = async () => {
        setError(null)
        try {
            const res = await fetch(`${PB_SERVER_ADDR}/api/admin/packages/builds/delete`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    Authorization: pb.authStore.token,
                },
                body: JSON.stringify({ buildId: build.build_id }),
            })
            const data = await res.json()
            if (!res.ok) {
                setError(data.error ?? 'Failed to delete build')
                return
            }
            setConfirmDelete(false)
            onChanged()
        } catch (err) {
            captureException('setup.buildHistory.delete', err)
            setError('Failed to delete build')
        }
    }

    return (
        <View className="px-4 py-3 gap-2">
            <View className="flex-row items-center justify-between gap-3">
                <View className="flex-1 gap-0.5">
                    <View className="flex-row gap-2 items-center flex-wrap">
                        <Text
                            className="text-foreground"
                            style={{ fontSize: 15, fontWeight: '600' }}
                        >
                            {build.pkg_slug}
                        </Text>
                        {build.version ? (
                            <Text className="text-muted-foreground" style={{ fontSize: 12 }}>
                                v{build.version}
                            </Text>
                        ) : null}
                        <BuildStatusBadge status={build.status} />
                        {isRevertMarker ? (
                            <Text style={{ fontSize: 11, color: `${mutedColor}99` }}>revert</Text>
                        ) : null}
                    </View>
                    <Text style={{ fontSize: 12, color: `${mutedColor}99` }}>
                        {formatWhen(build.created)}
                        {build.migrations_applied > 0
                            ? ` · ${build.migrations_applied} migration${build.migrations_applied === 1 ? '' : 's'}`
                            : ''}
                    </Text>
                </View>

                <BuildActions
                    canRevert={!isCurrent && !isSuperseded && !isRevertMarker}
                    canDelete={!isCurrent}
                    confirmRevert={confirmRevert}
                    confirmDelete={confirmDelete}
                    onRevert={() => {
                        setConfirmDelete(false)
                        setConfirmRevert(true)
                    }}
                    onDelete={() => {
                        setConfirmRevert(false)
                        setConfirmDelete(true)
                    }}
                    onCancel={() => {
                        setConfirmRevert(false)
                        setConfirmDelete(false)
                    }}
                    onConfirmRevert={doRevert}
                    onConfirmDelete={doDelete}
                    mutedColor={mutedColor}
                />
            </View>

            <RevertConfirmNotice
                isVisible={confirmRevert}
                invalidates={invalidates}
                mutedColor={mutedColor}
            />

            {error ? (
                <View className="rounded-md p-2 bg-danger-soft">
                    <Text className="text-xs text-danger">{error}</Text>
                </View>
            ) : null}
        </View>
    )
}

function RevertConfirmNotice({
    isVisible,
    invalidates,
    mutedColor,
}: {
    isVisible: boolean
    invalidates: BuildRecord[]
    mutedColor: string
}) {
    if (!isVisible) return null
    return (
        <View className="rounded-md p-2.5 gap-1" style={{ backgroundColor: `${mutedColor}12` }}>
            <Text className="text-foreground" style={{ fontSize: 12 }}>
                Reverting reverses the schema added since this build (your data is preserved) and
                restarts the server.
            </Text>
            {invalidates.length > 0 ? (
                <Text className="text-warning" style={{ fontSize: 12 }}>
                    This permanently invalidates {invalidates.length} newer build
                    {invalidates.length === 1 ? '' : 's'}:{' '}
                    {invalidates.map(b => `${b.pkg_slug} v${b.version}`).join(', ')}.
                </Text>
            ) : null}
        </View>
    )
}

function BuildActions({
    canRevert,
    canDelete,
    confirmRevert,
    confirmDelete,
    onRevert,
    onDelete,
    onCancel,
    onConfirmRevert,
    onConfirmDelete,
    mutedColor,
}: {
    canRevert: boolean
    canDelete: boolean
    confirmRevert: boolean
    confirmDelete: boolean
    onRevert: () => void
    onDelete: () => void
    onCancel: () => void
    onConfirmRevert: () => void
    onConfirmDelete: () => void
    mutedColor: string
}) {
    const primaryBg = useThemeColor('primary')
    const primaryFg = useThemeColor('primary-foreground')
    const dangerColor = useThemeColor('danger')
    const dangerFg = useThemeColor('danger-foreground')

    if (confirmRevert) {
        return (
            <View className="flex-row gap-1.5 items-center">
                <Pressable
                    onPress={onConfirmRevert}
                    className="px-3 py-1.5 rounded-md"
                    style={{ backgroundColor: primaryBg }}
                >
                    <Text style={{ fontSize: 12, fontWeight: '600', color: primaryFg }}>
                        Revert
                    </Text>
                </Pressable>
                <Pressable onPress={onCancel} className="p-1">
                    <X size={14} color={mutedColor} />
                </Pressable>
            </View>
        )
    }

    if (confirmDelete) {
        return (
            <View className="flex-row gap-1.5 items-center">
                <Pressable
                    onPress={onConfirmDelete}
                    className="px-3 py-1.5 rounded-md"
                    style={{ backgroundColor: dangerColor }}
                >
                    <Text style={{ fontSize: 12, fontWeight: '600', color: dangerFg }}>Delete</Text>
                </Pressable>
                <Pressable onPress={onCancel} className="p-1">
                    <X size={14} color={mutedColor} />
                </Pressable>
            </View>
        )
    }

    return (
        <View className="flex-row gap-2 items-center">
            {canRevert ? (
                <Pressable
                    onPress={onRevert}
                    className="flex-row gap-1 items-center px-2.5 py-1.5 rounded-md"
                    style={{ borderWidth: 1, borderColor: `${primaryBg}40` }}
                >
                    <RotateCcw size={13} color={primaryBg} />
                    <Text className="text-primary" style={{ fontSize: 12, fontWeight: '600' }}>
                        Revert
                    </Text>
                </Pressable>
            ) : null}
            {canDelete ? (
                <Pressable onPress={onDelete} className="p-1.5 rounded-md">
                    <Trash2 size={15} color={dangerColor} />
                </Pressable>
            ) : null}
        </View>
    )
}

const badgeClasses: Record<string, { bg: string; text: string }> = {
    current: { bg: 'bg-success-soft', text: 'text-success-soft-foreground' },
    available: { bg: 'bg-muted', text: 'text-muted-foreground' },
    superseded: { bg: 'bg-danger-soft', text: 'text-danger-soft-foreground' },
}

function BuildStatusBadge({ status }: { status: string }) {
    const variant = badgeClasses[status] ?? badgeClasses.available
    return (
        <View className={`px-2 py-0.5 rounded-full ${variant.bg}`}>
            <Text className={`text-[11px] font-semibold ${variant.text}`}>{status}</Text>
        </View>
    )
}
