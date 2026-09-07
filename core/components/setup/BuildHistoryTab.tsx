import { useLiveQuery } from '@tanstack/react-db'
import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { captureException } from '@tinycld/core/lib/errors'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonIcon, ButtonText } from '@tinycld/core/ui/button'
import { Dialog } from '@tinycld/core/ui/dialog'
import { History, RotateCcw, ShieldCheck, Trash2 } from 'lucide-react-native'
import type PocketBase from 'pocketbase'
import { type ReactNode, useState } from 'react'
import { ActivityIndicator, Pressable, Text, View } from 'react-native'
import { PageHeader, SlugTag } from './console-ui'
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
    const [jobId, setJobId] = useState<string | null>(null)

    // Live pbtsdb query: build rows are written server-side by the install/revert
    // pipeline, and pbtsdb's realtime subscription propagates those here — so a
    // revert or delete reflects without a manual refetch.
    const [pkgBuildCollection] = useStore('pkg_build')
    const { data: rows = [], isLoading } = useLiveQuery(
        query => query.from({ pkg_build: pkgBuildCollection }),
        []
    )
    const builds = [...(rows as BuildRecord[])].sort((a, b) =>
        (b.created ?? '').localeCompare(a.created ?? '')
    )

    if (!isVisible) return null

    // The builds newer than a given one are the ones a revert to it will
    // invalidate — surfaced in the confirm dialog so the operator sees the cost.
    const newerThan = (build: BuildRecord) =>
        builds.filter(b => b.created > build.created && b.action === 'install')

    return (
        <View className="gap-6">
            <PageHeader
                title="Build History"
                subtitle="Every successful install is saved as a restorable build. Reverting reverses all schema changes made since that point — your data is preserved, but newer builds are permanently invalidated."
            />

            <InstallProgressModal
                isVisible={jobId !== null}
                jobId={jobId}
                authToken={pb.authStore.token}
                onClose={() => setJobId(null)}
                onComplete={() => {}}
            />

            <BuildTimeline
                builds={builds}
                isLoading={isLoading}
                mutedColor={mutedColor}
                pb={pb}
                newerThan={newerThan}
                onJobStarted={setJobId}
            />
        </View>
    )
}

function BuildTimeline({
    builds,
    isLoading,
    mutedColor,
    pb,
    newerThan,
    onJobStarted,
}: {
    builds: BuildRecord[]
    isLoading: boolean
    mutedColor: string
    pb: PocketBase
    newerThan: (b: BuildRecord) => BuildRecord[]
    onJobStarted: (jobId: string) => void
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
            <View className="p-10 items-center rounded-xl border gap-2 bg-surface-secondary border-border">
                <History size={32} color={`${mutedColor}60`} />
                <Text className="text-muted-foreground" style={{ fontSize: 15 }}>
                    No builds yet. Installing a package saves its first build.
                </Text>
            </View>
        )
    }

    return (
        <View>
            {builds.map((build, i) => (
                <BuildTimelineItem
                    key={build.id}
                    build={build}
                    isLast={i === builds.length - 1}
                    invalidates={newerThan(build)}
                    pb={pb}
                    onJobStarted={onJobStarted}
                />
            ))}
        </View>
    )
}

function BuildTimelineItem({
    build,
    isLast,
    invalidates,
    pb,
    onJobStarted,
}: {
    build: BuildRecord
    isLast: boolean
    invalidates: BuildRecord[]
    pb: PocketBase
    onJobStarted: (jobId: string) => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const successColor = useThemeColor('success')
    const successSoft = useThemeColor('success-soft')
    const borderColor = useThemeColor('border')
    const [showRevert, setShowRevert] = useState(false)
    const [showDelete, setShowDelete] = useState(false)
    const [error, setError] = useState<string | null>(null)

    const isCurrent = build.status === 'current'
    const isSuperseded = build.status === 'superseded'
    const isRevertMarker = build.action === 'revert'
    const canRevert = !isCurrent && !isSuperseded && !isRevertMarker
    const canDelete = !isCurrent

    const doRevert = async () => {
        setError(null)
        const res = await fetch(`${PB_SERVER_ADDR}/api/admin/packages/revert`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', Authorization: pb.authStore.token },
            body: JSON.stringify({ buildId: build.build_id }),
        })
        const data = await res.json()
        if (!res.ok) throw new Error(data.error ?? 'Failed to start revert')
        setShowRevert(false)
        onJobStarted(data.jobId)
    }

    const doDelete = async () => {
        setError(null)
        const res = await fetch(`${PB_SERVER_ADDR}/api/admin/packages/builds/delete`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', Authorization: pb.authStore.token },
            body: JSON.stringify({ buildId: build.build_id }),
        })
        const data = await res.json()
        if (!res.ok) throw new Error(data.error ?? 'Failed to delete build')
        setShowDelete(false)
        // The list reflects the deletion via pbtsdb realtime — no manual refetch.
    }

    // node colors: current = filled success w/ ring; superseded/revert = hollow muted.
    const nodeFill = isCurrent ? successColor : 'transparent'
    const nodeBorder = isCurrent ? successColor : `${mutedColor}80`
    const dimmed = isSuperseded || isRevertMarker

    return (
        <View testID={`build-row-${build.build_id}`} className="flex-row gap-4">
            <View className="items-center" style={{ width: 16 }}>
                <View
                    style={{
                        width: 14,
                        height: 14,
                        borderRadius: 7,
                        marginTop: 22,
                        backgroundColor: nodeFill,
                        borderWidth: 2,
                        borderColor: nodeBorder,
                        ...(isCurrent
                            ? { shadowColor: successColor, shadowOpacity: 1, shadowRadius: 0 }
                            : {}),
                    }}
                />
                {isCurrent && (
                    <View
                        style={{
                            position: 'absolute',
                            top: 18,
                            width: 22,
                            height: 22,
                            borderRadius: 11,
                            backgroundColor: successSoft,
                        }}
                    />
                )}
                {!isLast && (
                    <View
                        className="flex-1"
                        style={{ width: 2, marginTop: 2, backgroundColor: borderColor }}
                    />
                )}
            </View>

            <View
                className="flex-1 mb-4 rounded-xl border bg-surface-secondary border-border p-4 gap-2"
                style={dimmed ? { opacity: 0.62 } : undefined}
            >
                <View className="flex-row items-center justify-between gap-3">
                    <View className="flex-1 gap-1.5">
                        <View className="flex-row gap-2 items-center flex-wrap">
                            <SlugTag>
                                {build.pkg_slug}
                                {build.version ? ` · v${build.version}` : ''}
                            </SlugTag>
                            <BuildStatusBadge status={build.status} />
                            {isRevertMarker ? (
                                <Text style={{ fontSize: 11, color: `${mutedColor}99` }}>
                                    revert
                                </Text>
                            ) : null}
                        </View>
                        <Text
                            style={{
                                fontSize: 12,
                                color: `${mutedColor}cc`,
                                fontFamily: 'monospace',
                            }}
                        >
                            {formatWhen(build.created)}
                            {build.migrations_applied > 0
                                ? ` · ${build.migrations_applied} migration${build.migrations_applied === 1 ? '' : 's'}`
                                : ''}
                        </Text>
                    </View>

                    <View className="flex-row gap-2 items-center">
                        {canRevert ? (
                            <Button onPress={() => setShowRevert(true)} size="sm" variant="outline">
                                <ButtonIcon as={RotateCcw} />
                                <ButtonText>Revert here</ButtonText>
                            </Button>
                        ) : null}
                        {canDelete ? (
                            <DeleteBuildButton onPress={() => setShowDelete(true)} />
                        ) : null}
                    </View>
                </View>

                {error ? (
                    <View className="rounded-md p-2 bg-danger-soft">
                        <Text className="text-xs text-danger">{error}</Text>
                    </View>
                ) : null}
            </View>

            <RevertModal
                isOpen={showRevert}
                build={build}
                invalidates={invalidates}
                onClose={() => setShowRevert(false)}
                onConfirm={doRevert}
            />
            <DeleteBuildModal
                isOpen={showDelete}
                build={build}
                onClose={() => setShowDelete(false)}
                onConfirm={doDelete}
            />
        </View>
    )
}

function DeleteBuildButton({ onPress }: { onPress: () => void }) {
    const dangerColor = useThemeColor('danger')
    return (
        <Pressable onPress={onPress} className="p-2 rounded-lg">
            <Trash2 size={15} color={dangerColor} />
        </Pressable>
    )
}

function RevertModal({
    isOpen,
    build,
    invalidates,
    onClose,
    onConfirm,
}: {
    isOpen: boolean
    build: BuildRecord
    invalidates: BuildRecord[]
    onClose: () => void
    onConfirm: () => Promise<void>
}) {
    const warningColor = useThemeColor('warning')
    const successColor = useThemeColor('success')
    const [submitting, setSubmitting] = useState(false)
    const [error, setError] = useState<string | null>(null)

    if (!isOpen) return null

    const close = () => {
        setError(null)
        onClose()
    }

    const confirm = async () => {
        setSubmitting(true)
        setError(null)
        try {
            await onConfirm()
        } catch (err) {
            captureException('setup.buildHistory.revert', err)
            setError(err instanceof Error ? err.message : 'Failed to start revert')
        } finally {
            setSubmitting(false)
        }
    }

    return (
        <Dialog
            isOpen
            onClose={close}
            size="lg"
            title="Revert to this build"
            header={
                <BuildDialogHeader
                    icon={<RotateCcw size={20} color={warningColor} />}
                    iconClassName="bg-warning-soft"
                    title="Revert to this build"
                >
                    Roll the deployment back to <BuildName build={build} />. This reverses the
                    schema added since it and restarts the server.
                </BuildDialogHeader>
            }
        >
            <Dialog.Body>
                <InvalidatedBuilds builds={invalidates} />
                <View className="flex-row gap-2 items-center px-3 py-2.5 rounded-lg bg-success-soft">
                    <ShieldCheck size={16} color={successColor} />
                    <Text className="text-success-soft-foreground" style={{ fontSize: 13 }}>
                        Your data is preserved. A backup is taken before reverting.
                    </Text>
                </View>
            </Dialog.Body>

            <DialogError message={error} />

            <Dialog.Footer>
                <Dialog.CancelButton onPress={close} isDisabled={submitting} />
                <Dialog.ActionButton
                    label={revertLabel(submitting, invalidates.length)}
                    onPress={confirm}
                    isDisabled={submitting}
                    isDestructive
                />
            </Dialog.Footer>
        </Dialog>
    )
}

function DeleteBuildModal({
    isOpen,
    build,
    onClose,
    onConfirm,
}: {
    isOpen: boolean
    build: BuildRecord
    onClose: () => void
    onConfirm: () => Promise<void>
}) {
    const dangerColor = useThemeColor('danger')
    const [submitting, setSubmitting] = useState(false)
    const [error, setError] = useState<string | null>(null)

    if (!isOpen) return null

    const close = () => {
        setError(null)
        onClose()
    }

    const confirm = async () => {
        setSubmitting(true)
        setError(null)
        try {
            await onConfirm()
        } catch (err) {
            captureException('setup.buildHistory.delete', err)
            setError(err instanceof Error ? err.message : 'Failed to delete build')
        } finally {
            setSubmitting(false)
        }
    }

    return (
        <Dialog
            isOpen
            onClose={close}
            size="lg"
            title="Delete this build archive"
            header={
                <BuildDialogHeader
                    icon={<Trash2 size={20} color={dangerColor} />}
                    iconClassName="bg-danger-soft"
                    title="Delete this build archive"
                >
                    Removes the archived build for <BuildName build={build} />. You won't be able to
                    revert to it afterward.
                </BuildDialogHeader>
            }
        >
            <DialogError message={error} />

            <Dialog.Footer>
                <Dialog.CancelButton onPress={close} isDisabled={submitting} />
                <Dialog.ActionButton
                    label={submitting ? 'Deleting…' : 'Delete build'}
                    onPress={confirm}
                    isDisabled={submitting}
                    isDestructive
                />
            </Dialog.Footer>
        </Dialog>
    )
}

function revertLabel(submitting: boolean, invalidated: number) {
    if (submitting) return 'Reverting…'
    if (invalidated > 0) return `Revert & invalidate ${invalidated}`
    return 'Revert'
}

// BuildDialogHeader replaces the Dialog's default title row so the confirm
// keeps its icon tile beside the title; the children are the description.
function BuildDialogHeader({
    icon,
    iconClassName,
    title,
    children,
}: {
    icon: ReactNode
    iconClassName: string
    title: string
    children: ReactNode
}) {
    return (
        <View className="flex-row gap-3 items-start px-5 pt-5 pb-3">
            <View className={`w-10 h-10 rounded-xl items-center justify-center ${iconClassName}`}>
                {icon}
            </View>
            <View className="flex-1 gap-1">
                <Text className="text-foreground" style={{ fontSize: 18, fontWeight: '600' }}>
                    {title}
                </Text>
                <Text className="text-muted-foreground" style={{ fontSize: 14 }}>
                    {children}
                </Text>
            </View>
        </View>
    )
}

function BuildName({ build }: { build: BuildRecord }) {
    return (
        <Text style={{ fontFamily: 'monospace', fontWeight: '600' }}>
            {build.pkg_slug}
            {build.version ? ` v${build.version}` : ''}
        </Text>
    )
}

function InvalidatedBuilds({ builds }: { builds: BuildRecord[] }) {
    if (builds.length === 0) return null
    return (
        <View className="rounded-xl p-4 gap-2 bg-danger-soft">
            <Text
                className="text-danger"
                style={{
                    fontSize: 12,
                    fontWeight: '600',
                    textTransform: 'uppercase',
                    letterSpacing: 0.5,
                }}
            >
                Permanently invalidates {builds.length} newer build
                {builds.length === 1 ? '' : 's'}
            </Text>
            {builds.map(b => (
                <Text
                    key={b.id}
                    className="text-danger"
                    style={{ fontSize: 13, fontFamily: 'monospace' }}
                >
                    − {b.pkg_slug} v{b.version}
                </Text>
            ))}
        </View>
    )
}

// Pinned between the body and the footer so a failure is visible without scrolling.
function DialogError({ message }: { message: string | null }) {
    if (!message) return null
    return (
        <View className="px-5 pb-4">
            <View className="rounded-lg p-2 bg-danger-soft">
                <Text className="text-danger" style={{ fontSize: 12 }}>
                    {message}
                </Text>
            </View>
        </View>
    )
}

const badgeClasses: Record<string, { bg: string; text: string }> = {
    current: { bg: 'bg-success-soft', text: 'text-success-soft-foreground' },
    available: { bg: 'bg-muted', text: 'text-muted-foreground' },
    superseded: { bg: 'bg-danger-soft', text: 'text-danger-soft-foreground' },
}

const badgeLabels: Record<string, string> = {
    available: 'restorable',
}

function BuildStatusBadge({ status }: { status: string }) {
    const variant = badgeClasses[status] ?? badgeClasses.available
    const label = badgeLabels[status] ?? status
    return (
        <View className={`px-2 py-0.5 rounded-full ${variant.bg}`}>
            <Text className={`text-[11px] font-semibold ${variant.text}`}>{label}</Text>
        </View>
    )
}
