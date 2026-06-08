import { DragHandle } from '@tinycld/core/components/DragHandle'
import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { captureException } from '@tinycld/core/lib/errors'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonIcon, ButtonText } from '@tinycld/core/ui/button'
import { Divider } from '@tinycld/core/ui/divider'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { Modal, ModalBackdrop, ModalContent } from '@tinycld/core/ui/modal'
import { Switch } from '@tinycld/core/ui/switch'
import {
    AlertTriangle,
    CircleCheck,
    CircleX,
    Download,
    Loader,
    Package,
    Pencil,
    Plus,
    ShieldCheck,
    Trash2,
    X,
} from 'lucide-react-native'
import type PocketBase from 'pocketbase'
import { useCallback, useEffect, useState } from 'react'
import {
    ActivityIndicator,
    Pressable,
    TextInput as RNTextInput,
    StyleSheet,
    Text,
    View,
} from 'react-native'
import DraggableFlatList, {
    type RenderItemParams,
    ScaleDecorator,
} from 'react-native-draggable-flatlist'
import { PageHeader, SectionLabel, SlugTag } from './console-ui'
import { InstallProgressModal } from './InstallProgressModal'
import { PackageStatusBadge } from './PackageStatusBadge'
import {
    type CompatViolation,
    type PackageVersionInfo,
    usePackageVersions,
} from './use-package-versions'
import { ConfirmChangesModal } from './version-apply'
import {
    buildVersionOptions,
    ChangeFlag,
    type RowDirection,
    RowVersionSelect,
    stagedDirection,
} from './version-controls'

interface PkgRecord {
    id: string
    name: string
    slug: string
    npm_package: string
    version: string
    status: string
    icon: string
    description: string
    nav_order: number
    has_server: boolean
}

const registerSchema = z.object({
    name: z.string().min(1, 'Required'),
    slug: z
        .string()
        .min(1, 'Required')
        .regex(/^[a-z0-9][a-z0-9-]*$/, 'Lowercase letters, numbers, hyphens'),
    npm_package: z.string().min(1, 'Required'),
    description: z.string(),
})

interface PackageManagerProps {
    pb: PocketBase
    // Gates the version-discovery fetch so it only runs when this screen is shown
    // (the dashboard keeps every tab mounted). Defaults true for any standalone use.
    isVisible?: boolean
}

export function PackageManager({ pb, isVisible = true }: PackageManagerProps) {
    const [packages, setPackages] = useState<PkgRecord[]>([])
    const [isLoading, setIsLoading] = useState(true)
    const [showRegister, setShowRegister] = useState(false)
    const [showInstall, setShowInstall] = useState(false)
    const [editingId, setEditingId] = useState<string | null>(null)
    const [installJobId, setInstallJobId] = useState<string | null>(null)
    const [confirmOpen, setConfirmOpen] = useState(false)

    // Version discovery + the staging/compat/apply solver. Lives alongside the
    // lifecycle list so a package's version sits on its own row.
    const vm = usePackageVersions(pb, isVisible)
    const versionBySlug = new Map(vm.versions.map(v => [v.slug, v]))

    const fetchPackages = useCallback(async () => {
        setIsLoading(true)
        try {
            const records = await pb
                .collection('pkg_registry')
                .getFullList<PkgRecord>({ sort: 'nav_order,name' })
            setPackages(records)
        } catch (err) {
            captureException('Failed to fetch packages', err)
        } finally {
            setIsLoading(false)
        }
    }, [pb])

    useEffect(() => {
        fetchPackages()
    }, [fetchPackages])

    const handleInstallStarted = useCallback((jobId: string) => {
        setShowInstall(false)
        setInstallJobId(jobId)
    }, [])

    const handleUninstallStarted = useCallback((jobId: string) => {
        setInstallJobId(jobId)
    }, [])

    return (
        <View className="gap-6">
            <PageHeader
                title="Packages"
                subtitle="Features installed on this deployment. Toggle availability, reorder the sidebar, change versions, install from npm, or register a new source."
                actions={
                    <>
                        <Button onPress={() => setShowRegister(true)} size="sm" variant="outline">
                            <ButtonIcon as={Plus} />
                            <ButtonText>Register</ButtonText>
                        </Button>
                        <Button onPress={() => setShowInstall(true)} size="sm">
                            <ButtonIcon as={Download} />
                            <ButtonText>Install package</ButtonText>
                        </Button>
                    </>
                }
            />

            <InstallPackageModal
                isOpen={showInstall}
                pb={pb}
                onClose={() => setShowInstall(false)}
                onStarted={handleInstallStarted}
            />

            <InstallProgressModal
                isVisible={installJobId !== null}
                jobId={installJobId}
                authToken={pb.authStore.token}
                onClose={() => {
                    setInstallJobId(null)
                    fetchPackages()
                }}
                onComplete={fetchPackages}
            />

            <RegisterPackageModal
                isOpen={showRegister}
                pb={pb}
                onClose={() => setShowRegister(false)}
                onCreated={() => {
                    setShowRegister(false)
                    fetchPackages()
                }}
            />

            <View className="gap-3">
                <View className="flex-row items-center justify-between">
                    <SectionLabel>Installed · drag to reorder sidebar</SectionLabel>
                    <StageActions
                        pendingCount={vm.pendingChanges.length}
                        onSelectAll={vm.selectAllUpdates}
                        onClear={vm.clearSelection}
                    />
                </View>
                <View className="rounded-xl border overflow-hidden bg-surface-secondary border-border">
                    <PackageList
                        packages={packages}
                        isLoading={isLoading}
                        pb={pb}
                        editingId={editingId}
                        onEdit={setEditingId}
                        onUpdated={fetchPackages}
                        onUninstallStarted={handleUninstallStarted}
                        versionBySlug={versionBySlug}
                        targets={vm.targets}
                        onSetTarget={vm.setTarget}
                    />
                    <ViolationList violations={vm.violations} isChecking={vm.isChecking} />
                    <ApplyFooter
                        pendingCount={vm.pendingChanges.length}
                        violationCount={vm.violations.length}
                        isChecking={vm.isChecking}
                        canApply={vm.canApply}
                        onApply={() => setConfirmOpen(true)}
                    />
                </View>
            </View>

            <ConfirmChangesModal
                isOpen={confirmOpen}
                pendingChanges={vm.pendingChanges}
                hasDowngrade={vm.hasDowngrade}
                fetchDropReport={vm.fetchDropReport}
                onCancel={() => setConfirmOpen(false)}
                onConfirm={async () => {
                    await vm.applyChanges()
                    setConfirmOpen(false)
                }}
            />

            <InstallProgressModal
                isVisible={vm.applyJobId !== null}
                jobId={vm.applyJobId}
                authToken={pb.authStore.token}
                onClose={() => {
                    vm.onApplyComplete()
                    fetchPackages()
                }}
                onComplete={() => {
                    vm.refresh()
                    fetchPackages()
                }}
            />
        </View>
    )
}

function PackageList({
    packages,
    isLoading,
    pb,
    editingId,
    onEdit,
    onUpdated,
    onUninstallStarted,
    versionBySlug,
    targets,
    onSetTarget,
}: {
    packages: PkgRecord[]
    isLoading: boolean
    pb: PocketBase
    editingId: string | null
    onEdit: (id: string | null) => void
    onUpdated: () => void
    onUninstallStarted: (jobId: string) => void
    versionBySlug: Map<string, PackageVersionInfo>
    targets: Record<string, string>
    onSetTarget: (slug: string, version: string) => void
}) {
    const surfaceBg = useThemeColor('surface-secondary')
    const accentColor = useThemeColor('accent')
    const borderColor = useThemeColor('border')
    const mutedColor = useThemeColor('muted-foreground')

    const pkgMap = new Map(packages.map(p => [p.id, p]))

    const handleDragEnd = useCallback(
        async ({ data }: { data: PkgRecord[] }) => {
            try {
                await Promise.all(
                    data.map((pkg, i) =>
                        pb.collection('pkg_registry').update(pkg.id, { nav_order: i * 10 })
                    )
                )
                onUpdated()
            } catch (err) {
                captureException('Failed to save package order', err)
            }
        },
        [pb, onUpdated]
    )

    function renderItem({ item, drag, isActive }: RenderItemParams<PkgRecord>) {
        const pkg = pkgMap.get(item.id) ?? item
        const info = versionBySlug.get(pkg.slug)
        const target = targets[pkg.slug]
        // A row with a staged version change locks its lifecycle controls
        // (toggle/drag/uninstall) until the change is applied or cleared, so an
        // immediate mutation can't collide with the pending transaction.
        const isStaged = info ? stagedDirection(info, target) !== 'none' : false

        return (
            <ScaleDecorator activeScale={1}>
                <View
                    style={{
                        backgroundColor: isActive
                            ? `${accentColor}20`
                            : isStaged
                              ? `${accentColor}1f`
                              : surfaceBg,
                        borderBottomWidth: StyleSheet.hairlineWidth,
                        borderColor,
                    }}
                >
                    <View className="flex-row items-center gap-3 px-4 py-3.5">
                        <DragHandle
                            drag={drag}
                            disabled={isActive || isStaged}
                            color={mutedColor}
                        />
                        <View className="w-10 h-10 rounded-xl items-center justify-center bg-surface border border-border">
                            <Package size={18} color={mutedColor} />
                        </View>
                        <View className="flex-1 gap-0.5">
                            <View className="flex-row gap-2 items-center">
                                <Text
                                    style={{
                                        fontSize: 15,
                                        fontWeight: '600',
                                        color: isActive ? mutedColor : undefined,
                                    }}
                                    className="text-foreground"
                                >
                                    {pkg.name}
                                </Text>
                                <PackageStatusBadge status={pkg.status} />
                                {info?.hasUpdate && !isStaged ? (
                                    <PackageStatusBadge status="update-available" />
                                ) : null}
                            </View>
                            <View className="flex-row gap-2 items-center flex-wrap">
                                <SlugTag>{pkg.slug}</SlugTag>
                                {pkg.description ? (
                                    <Text
                                        className="text-muted-foreground"
                                        style={{ fontSize: 13 }}
                                        numberOfLines={1}
                                    >
                                        {pkg.description}
                                    </Text>
                                ) : null}
                            </View>
                        </View>
                        <RowVersion info={info} target={target} onSetTarget={onSetTarget} />
                        <PackageActions
                            pkg={pkg}
                            pb={pb}
                            isEditing={editingId === pkg.id}
                            isLocked={isStaged}
                            onEdit={() => onEdit(editingId === pkg.id ? null : pkg.id)}
                            onUpdated={onUpdated}
                            onUninstallStarted={onUninstallStarted}
                        />
                    </View>
                    <EditPackageForm
                        isVisible={editingId === pkg.id && !isStaged}
                        pkg={pkg}
                        pb={pb}
                        onClose={() => onEdit(null)}
                        onUpdated={onUpdated}
                    />
                </View>
            </ScaleDecorator>
        )
    }

    const keyExtractor = useCallback((item: PkgRecord) => item.id, [])

    if (isLoading) {
        return (
            <View className="p-8 items-center">
                <ActivityIndicator size="large" color={mutedColor} />
            </View>
        )
    }

    if (packages.length === 0) {
        return (
            <View className="p-8 items-center gap-2">
                <Package size={32} color={`${mutedColor}60`} />
                <Text className="text-muted-foreground" style={{ fontSize: 15 }}>
                    No packages installed yet
                </Text>
            </View>
        )
    }

    return (
        <DraggableFlatList
            data={packages}
            keyExtractor={keyExtractor}
            onDragEnd={handleDragEnd}
            renderItem={renderItem}
            scrollEnabled={false}
            activationDistance={1}
        />
    )
}

// RowVersion is the per-row version cell. A package with >1 available version
// gets the interactive picker + change flag; a bundled / single-version package
// (Core, unmanaged sources) shows just its static version, no dropdown — there's
// nothing to stage.
function RowVersion({
    info,
    target,
    onSetTarget,
}: {
    info?: PackageVersionInfo
    target?: string
    onSetTarget: (slug: string, version: string) => void
}) {
    if (!info) {
        return null
    }

    const options = buildVersionOptions(info)
    const direction: RowDirection = stagedDirection(info, target)

    if (options.length <= 1) {
        return (
            <View style={{ width: 150 }} className="items-end">
                <Text
                    className="text-muted-foreground"
                    style={{ fontFamily: 'monospace', fontSize: 13 }}
                >
                    v{info.current || '—'}
                </Text>
            </View>
        )
    }

    return (
        <View style={{ width: 150 }} className="gap-1">
            <RowVersionSelect
                options={options}
                value={target ?? info.current}
                direction={direction}
                onChange={v => onSetTarget(info.slug, v)}
            />
            <ChangeFlag direction={direction} />
        </View>
    )
}

function PackageActions({
    pkg,
    pb,
    isEditing,
    isLocked,
    onEdit,
    onUpdated,
    onUninstallStarted,
}: {
    pkg: PkgRecord
    pb: PocketBase
    isEditing: boolean
    // True when this package has a staged version change — its toggle and
    // uninstall are disabled until the change is applied or cleared, so an
    // immediate mutation can't race the pending transaction.
    isLocked: boolean
    onEdit: () => void
    onUpdated: () => void
    onUninstallStarted: (jobId: string) => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const dangerColor = useThemeColor('danger')
    const primaryBg = useThemeColor('primary')
    const [isToggling, setIsToggling] = useState(false)
    const [showUninstall, setShowUninstall] = useState(false)

    const isEnabled = pkg.status !== 'disabled'
    const isInstalled = pkg.status === 'installed'

    const toggleStatus = async () => {
        setIsToggling(true)
        try {
            const isBundled = pkg.status === 'bundled'
            const newStatus = isEnabled ? 'disabled' : isBundled ? 'bundled' : 'installed'
            await pb.collection('pkg_registry').update(pkg.id, { status: newStatus })
            onUpdated()
        } catch (err) {
            captureException('Failed to toggle package status', err)
        } finally {
            setIsToggling(false)
        }
    }

    const handleUninstall = async () => {
        const response = await fetch(`${PB_SERVER_ADDR}/api/admin/packages/uninstall`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                Authorization: pb.authStore.token,
            },
            body: JSON.stringify({ slug: pkg.slug }),
        })
        const data = await response.json()
        if (!response.ok) {
            throw new Error(data.error ?? 'Failed to start uninstall')
        }
        setShowUninstall(false)
        onUninstallStarted(data.jobId)
    }

    return (
        <View
            className="flex-row gap-2 items-center"
            style={isLocked ? { opacity: 0.5 } : undefined}
        >
            <Switch
                value={isEnabled}
                onValueChange={toggleStatus}
                disabled={isToggling || isLocked}
            />
            <Pressable
                onPress={onEdit}
                disabled={isLocked}
                className="p-2 rounded-lg"
                style={{ backgroundColor: isEditing ? `${primaryBg}15` : 'transparent' }}
            >
                <Pencil size={15} color={isEditing ? primaryBg : mutedColor} />
            </Pressable>
            {isInstalled ? (
                <Pressable
                    onPress={() => setShowUninstall(true)}
                    disabled={isLocked}
                    className="p-2 rounded-lg"
                >
                    <Trash2 size={15} color={dangerColor} />
                </Pressable>
            ) : null}
            <UninstallModal
                isOpen={showUninstall}
                name={pkg.name}
                slug={pkg.slug}
                onClose={() => setShowUninstall(false)}
                onConfirm={handleUninstall}
            />
        </View>
    )
}

// StageActions: the "stage all updates" / "clear" controls in the section header,
// shown alongside the package list. Mirrors the old Versions tab's selection bar.
function StageActions({
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
                <Button onPress={onClear} size="sm" variant="ghost">
                    <ButtonText>Clear ({pendingCount})</ButtonText>
                </Button>
            )}
            <Button onPress={onSelectAll} size="sm" variant="outline">
                <ButtonText>Stage all updates</ButtonText>
            </Button>
        </View>
    )
}

// ViolationList: the compatibility panel between the rows and the apply footer,
// inside the same bordered list, so a conflict reads as part of the staged set.
function ViolationList({
    violations,
    isChecking,
}: {
    violations: CompatViolation[]
    isChecking: boolean
}) {
    const dangerColor = useThemeColor('danger')
    if (isChecking || violations.length === 0) return null
    return (
        <View className="p-4 gap-2.5 bg-danger-soft border-t border-border">
            {violations.map(v => (
                <View key={`${v.package}:${v.requires}`} className="flex-row gap-2.5 items-start">
                    <AlertTriangle size={16} color={dangerColor} style={{ marginTop: 2 }} />
                    <Text className="text-danger flex-1" style={{ fontSize: 13.5 }}>
                        <Text style={{ fontFamily: 'monospace', fontWeight: '600' }}>
                            {v.package}
                        </Text>{' '}
                        requires{' '}
                        <Text style={{ fontFamily: 'monospace' }}>
                            {v.requires} {v.range}
                        </Text>{' '}
                        — found {v.found || 'not installed'}
                    </Text>
                </View>
            ))}
        </View>
    )
}

// ApplyFooter: the list's status bar — a live compatibility verdict on the left,
// the apply action on the right. Renders only once something is staged.
function ApplyFooter({
    pendingCount,
    violationCount,
    isChecking,
    canApply,
    onApply,
}: {
    pendingCount: number
    violationCount: number
    isChecking: boolean
    canApply: boolean
    onApply: () => void
}) {
    if (pendingCount === 0) return null
    const hasViolation = violationCount > 0
    return (
        <View
            className={`flex-row items-center gap-4 px-4 py-3.5 border-t border-border ${hasViolation ? 'bg-danger-soft' : 'bg-surface'}`}
        >
            <SolverStatus
                isChecking={isChecking}
                violationCount={violationCount}
                pendingCount={pendingCount}
            />
            <View className="flex-1" />
            <Button
                onPress={onApply}
                isDisabled={!canApply}
                size="sm"
                variant={hasViolation ? 'destructive' : 'default'}
            >
                <ButtonText>
                    Apply {pendingCount} change{pendingCount === 1 ? '' : 's'}
                </ButtonText>
            </Button>
        </View>
    )
}

function SolverStatus({
    isChecking,
    violationCount,
    pendingCount,
}: {
    isChecking: boolean
    violationCount: number
    pendingCount: number
}) {
    const infoColor = useThemeColor('link')
    const successColor = useThemeColor('success')
    const dangerColor = useThemeColor('danger')

    if (isChecking) {
        return (
            <View className="flex-row items-center gap-2">
                <Loader size={16} color={infoColor} />
                <Text style={{ color: infoColor, fontSize: 13.5 }}>Checking compatibility…</Text>
            </View>
        )
    }
    if (violationCount > 0) {
        return (
            <View className="flex-row items-center gap-2">
                <CircleX size={16} color={dangerColor} />
                <Text style={{ color: dangerColor, fontSize: 13.5 }}>
                    <Text style={{ fontWeight: '600' }}>
                        {violationCount} conflict{violationCount === 1 ? '' : 's'}
                    </Text>{' '}
                    · {pendingCount} staged
                </Text>
            </View>
        )
    }
    return (
        <View className="flex-row items-center gap-2">
            <CircleCheck size={16} color={successColor} />
            <Text style={{ color: successColor, fontSize: 13.5 }}>
                Compatible · {pendingCount} staged
            </Text>
        </View>
    )
}

// UninstallModal requires the operator to type the package slug before the
// destructive action unlocks — the same typed-confirmation guard used for
// downgrades, so an errant click can't drop a package's data.
function UninstallModal({
    isOpen,
    name,
    slug,
    onClose,
    onConfirm,
}: {
    isOpen: boolean
    name: string
    slug: string
    onClose: () => void
    onConfirm: () => Promise<void>
}) {
    const dangerColor = useThemeColor('danger')
    const fgColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const successColor = useThemeColor('success')
    const [typed, setTyped] = useState('')
    const [submitting, setSubmitting] = useState(false)
    const [error, setError] = useState<string | null>(null)

    if (!isOpen) return null

    const confirmEnabled = typed.trim() === slug && !submitting

    const close = () => {
        setTyped('')
        setError(null)
        onClose()
    }

    const confirm = async () => {
        setSubmitting(true)
        setError(null)
        try {
            await onConfirm()
            setTyped('')
        } catch (err) {
            captureException('setup.packages.uninstall', err)
            setError(err instanceof Error ? err.message : 'Failed to start uninstall')
        } finally {
            setSubmitting(false)
        }
    }

    return (
        <Modal isOpen onClose={close}>
            <ModalBackdrop />
            <ModalContent className="w-[460px] p-5 gap-4">
                <View className="flex-row gap-3 items-start">
                    <View className="w-10 h-10 rounded-xl items-center justify-center bg-danger-soft">
                        <Trash2 size={20} color={dangerColor} />
                    </View>
                    <View className="flex-1 gap-1">
                        <Text
                            className="text-foreground"
                            style={{ fontSize: 18, fontWeight: '600' }}
                        >
                            Uninstall {name}
                        </Text>
                        <Text className="text-muted-foreground" style={{ fontSize: 14 }}>
                            This removes the feature and runs its down-migrations. Records owned by
                            this package will be dropped.
                        </Text>
                    </View>
                </View>

                <View className="gap-2">
                    <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                        Type{' '}
                        <Text style={{ fontFamily: 'monospace', color: dangerColor }}>{slug}</Text>{' '}
                        to confirm
                    </Text>
                    <RNTextInput
                        value={typed}
                        onChangeText={setTyped}
                        autoCapitalize="none"
                        placeholder={slug}
                        placeholderTextColor={mutedColor}
                        className="rounded-lg border border-border px-3 py-2.5 bg-surface"
                        style={{ color: fgColor, fontSize: 14, fontFamily: 'monospace' }}
                    />
                </View>

                <View className="flex-row gap-2 items-center px-3 py-2.5 rounded-lg bg-success-soft">
                    <ShieldCheck size={16} color={successColor} />
                    <Text className="text-success-soft-foreground" style={{ fontSize: 13 }}>
                        A backup is taken before uninstalling.
                    </Text>
                </View>

                {error && (
                    <View className="rounded-lg p-2 bg-danger-soft">
                        <Text className="text-danger" style={{ fontSize: 12 }}>
                            {error}
                        </Text>
                    </View>
                )}

                <View className="flex-row gap-3 justify-end">
                    <Pressable onPress={close} className="px-3 py-2" disabled={submitting}>
                        <Text className="text-foreground" style={{ fontSize: 13 }}>
                            Cancel
                        </Text>
                    </Pressable>
                    <Button
                        onPress={confirm}
                        isDisabled={!confirmEnabled}
                        size="sm"
                        variant="destructive"
                    >
                        <ButtonText>{submitting ? 'Uninstalling…' : 'Uninstall'}</ButtonText>
                    </Button>
                </View>
            </ModalContent>
        </Modal>
    )
}

const editSchema = z.object({
    name: z.string().min(1, 'Required'),
    description: z.string(),
    icon: z.string(),
    nav_order: z.string(),
})

function EditPackageForm({
    isVisible,
    pkg,
    pb,
    onClose,
    onUpdated,
}: {
    isVisible: boolean
    pkg: PkgRecord
    pb: PocketBase
    onClose: () => void
    onUpdated: () => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const dangerColor = useThemeColor('danger')
    const borderColor = useThemeColor('border')
    const [isSaving, setIsSaving] = useState(false)
    const [saveError, setSaveError] = useState<string | null>(null)
    const [confirmRemove, setConfirmRemove] = useState(false)

    const handleRemove = async () => {
        try {
            await pb.collection('pkg_registry').delete(pkg.id)
            onClose()
            onUpdated()
        } catch (err) {
            setSaveError(err instanceof Error ? err.message : 'Failed to remove')
        }
    }

    const {
        control,
        handleSubmit,
        formState: { errors, isSubmitted, isDirty },
    } = useForm({
        resolver: zodResolver(editSchema),
        defaultValues: {
            name: pkg.name,
            description: pkg.description ?? '',
            icon: pkg.icon ?? '',
            nav_order: String(pkg.nav_order ?? 0),
        },
        mode: 'onChange',
    })

    const onSave = handleSubmit(async data => {
        setSaveError(null)
        setIsSaving(true)
        try {
            await pb.collection('pkg_registry').update(pkg.id, {
                name: data.name,
                description: data.description,
                icon: data.icon,
                nav_order: Number(data.nav_order) || 0,
            })
            onClose()
            onUpdated()
        } catch (err) {
            setSaveError(err instanceof Error ? err.message : 'Failed to update')
        } finally {
            setIsSaving(false)
        }
    })

    if (!isVisible) return null

    const saveEnabled = isDirty && !isSaving

    return (
        <View
            className="mx-3 mb-3 rounded-xl p-4 gap-3"
            style={{
                backgroundColor: `${borderColor}30`,
                borderWidth: StyleSheet.hairlineWidth,
                borderColor,
            }}
        >
            <View className="flex-row items-center justify-between">
                <SectionLabel>Edit package</SectionLabel>
                <SlugTag>
                    {pkg.slug}
                    {pkg.version ? ` · v${pkg.version}` : ''}
                </SlugTag>
            </View>

            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />
            {saveError && (
                <View className="rounded-md p-2.5 bg-danger-soft">
                    <Text className="text-xs text-danger">{saveError}</Text>
                </View>
            )}

            <View className="flex-row gap-3 flex-wrap">
                <View className="flex-1 min-w-[150px]">
                    <TextInput control={control} name="name" label="Name" />
                </View>
                <View style={{ width: 120 }}>
                    <TextInput control={control} name="icon" label="Icon" />
                </View>
                <View style={{ width: 72 }}>
                    <TextInput control={control} name="nav_order" label="Order" />
                </View>
            </View>
            <TextInput control={control} name="description" label="Description" />

            <Divider />

            <View className="flex-row items-center">
                {confirmRemove ? (
                    <View className="flex-row gap-1.5 items-center">
                        <Button onPress={handleRemove} size="sm" variant="destructive">
                            <ButtonText>Confirm remove</ButtonText>
                        </Button>
                        <Pressable onPress={() => setConfirmRemove(false)} className="p-1.5">
                            <X size={14} color={mutedColor} />
                        </Pressable>
                    </View>
                ) : (
                    <Pressable
                        onPress={() => setConfirmRemove(true)}
                        className="flex-row gap-1 items-center px-2.5 py-1.5 rounded-md"
                    >
                        <Trash2 size={13} color={dangerColor} />
                        <Text className="text-danger" style={{ fontWeight: '500', fontSize: 13 }}>
                            Remove
                        </Text>
                    </Pressable>
                )}
                <View className="flex-1" />
                <View className="flex-row gap-2 items-center">
                    <Pressable onPress={onClose} className="px-3 py-1.5 rounded-md">
                        <Text
                            className="text-muted-foreground"
                            style={{ fontWeight: '500', fontSize: 13 }}
                        >
                            Cancel
                        </Text>
                    </Pressable>
                    <Button onPress={onSave} isDisabled={!saveEnabled} size="sm">
                        <ButtonText>{isSaving ? 'Saving…' : 'Save'}</ButtonText>
                    </Button>
                </View>
            </View>
        </View>
    )
}

const registerDefaults = { name: '', slug: '', npm_package: '', description: '' }

function RegisterPackageModal({
    isOpen,
    pb,
    onClose,
    onCreated,
}: {
    isOpen: boolean
    pb: PocketBase
    onClose: () => void
    onCreated: () => void
}) {
    const [submitError, setSubmitError] = useState<string | null>(null)
    const [isCreating, setIsCreating] = useState(false)

    const {
        control,
        handleSubmit,
        reset,
        formState: { errors, isSubmitted },
    } = useForm({
        resolver: zodResolver(registerSchema),
        defaultValues: registerDefaults,
        mode: 'onChange',
    })

    const close = () => {
        reset(registerDefaults)
        setSubmitError(null)
        onClose()
    }

    const onSubmit = handleSubmit(async data => {
        setSubmitError(null)
        setIsCreating(true)
        try {
            await pb.collection('pkg_registry').create({
                ...data,
                status: 'available',
                nav_order: 0,
            })
            reset(registerDefaults)
            onCreated()
        } catch (err) {
            setSubmitError(err instanceof Error ? err.message : 'Failed to register')
        } finally {
            setIsCreating(false)
        }
    })

    if (!isOpen) return null

    return (
        <Modal isOpen onClose={close}>
            <ModalBackdrop />
            <ModalContent className="w-[520px] p-5 gap-4">
                <Text className="text-foreground" style={{ fontSize: 18, fontWeight: '600' }}>
                    Register a package source
                </Text>

                <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

                {submitError && (
                    <View className="rounded-md p-2.5 bg-danger-soft">
                        <Text className="text-xs text-danger">{submitError}</Text>
                    </View>
                )}

                <View className="flex-row gap-3 flex-wrap">
                    <View className="flex-1 min-w-[200px]">
                        <TextInput
                            control={control}
                            name="name"
                            label="Display name"
                            placeholder="Contacts"
                        />
                    </View>
                    <View className="flex-1 min-w-[200px]">
                        <TextInput
                            control={control}
                            name="slug"
                            label="Slug"
                            placeholder="contacts"
                            autoCapitalize="none"
                            hint="kebab-case"
                        />
                    </View>
                </View>
                <View className="flex-row gap-3 flex-wrap">
                    <View className="flex-1 min-w-[200px]">
                        <TextInput
                            control={control}
                            name="npm_package"
                            label="npm package / git URL"
                            placeholder="@tinycld/contacts"
                            autoCapitalize="none"
                        />
                    </View>
                    <View className="flex-1 min-w-[200px]">
                        <TextInput
                            control={control}
                            name="description"
                            label="Description"
                            placeholder="Optional"
                        />
                    </View>
                </View>

                <View className="flex-row gap-3 justify-end">
                    <Pressable onPress={close} className="px-3 py-2" disabled={isCreating}>
                        <Text className="text-foreground" style={{ fontSize: 13 }}>
                            Cancel
                        </Text>
                    </Pressable>
                    <Button onPress={onSubmit} isDisabled={isCreating} size="sm">
                        <ButtonText>{isCreating ? 'Registering…' : 'Register'}</ButtonText>
                    </Button>
                </View>
            </ModalContent>
        </Modal>
    )
}

const installSchema = z.object({
    npm_package: z.string().min(1, 'Required'),
})

function InstallPackageModal({
    isOpen,
    pb,
    onClose,
    onStarted,
}: {
    isOpen: boolean
    pb: PocketBase
    onClose: () => void
    onStarted: (jobId: string) => void
}) {
    const [submitError, setSubmitError] = useState<string | null>(null)
    const [isInstalling, setIsInstalling] = useState(false)

    const {
        control,
        handleSubmit,
        reset,
        watch,
        formState: { errors, isSubmitted },
    } = useForm({
        resolver: zodResolver(installSchema),
        defaultValues: { npm_package: '' },
        mode: 'onChange',
    })

    const npmValue = watch('npm_package')
    const showWarning = npmValue.length > 0 && !npmValue.startsWith('@tinycld/')

    const close = () => {
        reset({ npm_package: '' })
        setSubmitError(null)
        onClose()
    }

    const onSubmit = handleSubmit(async data => {
        setSubmitError(null)
        setIsInstalling(true)
        try {
            const response = await fetch(`${PB_SERVER_ADDR}/api/admin/packages/install`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    Authorization: pb.authStore.token,
                },
                body: JSON.stringify({ npmPackage: data.npm_package }),
            })
            const result = await response.json()
            if (!response.ok) {
                setSubmitError(result.error ?? 'Failed to start install')
                return
            }
            reset({ npm_package: '' })
            onStarted(result.jobId)
        } catch (err) {
            setSubmitError(err instanceof Error ? err.message : 'Failed to start install')
        } finally {
            setIsInstalling(false)
        }
    })

    if (!isOpen) return null

    return (
        <Modal isOpen onClose={close}>
            <ModalBackdrop />
            <ModalContent className="w-[520px] p-5 gap-4">
                <Text className="text-foreground" style={{ fontSize: 18, fontWeight: '600' }}>
                    Install from npm or git
                </Text>

                <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

                {submitError && (
                    <View className="rounded-md p-2.5 bg-danger-soft">
                        <Text className="text-xs text-danger">{submitError}</Text>
                    </View>
                )}

                <SecurityWarning isVisible={showWarning} />

                <TextInput
                    control={control}
                    name="npm_package"
                    label="Package source"
                    placeholder="@tinycld/contacts"
                    autoCapitalize="none"
                    hint="npm package name, version spec, or a git URL"
                />

                <View className="flex-row gap-3 justify-end">
                    <Pressable onPress={close} className="px-3 py-2" disabled={isInstalling}>
                        <Text className="text-foreground" style={{ fontSize: 13 }}>
                            Cancel
                        </Text>
                    </Pressable>
                    <Button onPress={onSubmit} isDisabled={isInstalling} size="sm">
                        <ButtonText>{isInstalling ? 'Starting…' : 'Install'}</ButtonText>
                    </Button>
                </View>
            </ModalContent>
        </Modal>
    )
}

function SecurityWarning({ isVisible }: { isVisible: boolean }) {
    const warningColor = useThemeColor('warning')
    if (!isVisible) return null
    return (
        <View className="flex-row gap-2 items-start rounded-lg p-3 bg-warning-soft">
            <AlertTriangle size={16} color={warningColor} style={{ marginTop: 1 }} />
            <Text className="text-warning-soft-foreground flex-1" style={{ fontSize: 13 }}>
                This package is not in the @tinycld/ scope. Only install packages you trust.
            </Text>
        </View>
    )
}
