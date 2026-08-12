import { eq } from '@tanstack/db'
import { EmptyState } from '@tinycld/core/components/EmptyState'
import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import type { RuleDraft } from '@tinycld/core/lib/automation/draft'
import { emptyDraft, recordToDraft } from '@tinycld/core/lib/automation/draft'
import { useAutomationCatalog } from '@tinycld/core/lib/automation/use-automation-catalog'
import { useRuleDraft } from '@tinycld/core/lib/automation/use-rule-draft'
import { useRuleMutations } from '@tinycld/core/lib/automation/use-rule-mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { BottomDrawer } from '@tinycld/core/ui/bottom-drawer'
import { Button, ButtonSpinner, ButtonText } from '@tinycld/core/ui/button'
import { Modal, ModalBackdrop, ModalContent } from '@tinycld/core/ui/modal'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { Switch } from '@tinycld/core/ui/switch'
import { X } from 'lucide-react-native'
import { useRef } from 'react'
import { ActivityIndicator, ScrollView, Text, View } from 'react-native'
import { ActionsCard } from './ActionsCard'
import { ConditionsCard } from './ConditionsCard'
import { DryRunPanel } from './DryRunPanel'
import { TriggerCard } from './TriggerCard'

export interface RuleBuilderProps {
    isOpen: boolean
    onClose: () => void
    scope: 'personal' | 'org'
    ruleId?: string
    presetPkg?: string
}

// Loads the record being edited (if any). Returns `isReady: true` for the
// create case (nothing to load) so the caller doesn't have to special-case
// it, and only actually queries `rules` when a ruleId is present.
function useEditingRecord(ruleId: string | undefined) {
    const [rulesCollection] = useStore('rules')
    const { data, isReady } = useOrgLiveQuery(
        query => {
            if (!ruleId) return null
            return query.from({ r: rulesCollection }).where(({ r }) => eq(r.id, ruleId))
        },
        [ruleId]
    )
    if (!ruleId) return { record: null, isReady: true }
    return { record: data?.[0] ?? null, isReady }
}

// presetPkg (Task 4 plumbing) only narrows the trigger picker's package group
// inside TriggerCard's menu — it isn't a trigger ref itself, so it plays no
// part in seeding the draft.
function initialDraft(
    scope: 'personal' | 'org',
    record: ReturnType<typeof useEditingRecord>['record']
): RuleDraft {
    return record ? recordToDraft(record) : emptyDraft(scope)
}

function BuilderContent({
    onClose,
    scope,
    ruleId,
    presetPkg,
    sessionKey,
}: Omit<RuleBuilderProps, 'isOpen'> & { sessionKey: number }) {
    const { record, isReady: recordReady } = useEditingRecord(ruleId)
    const { catalog, isReady: catalogReady } = useAutomationCatalog()
    const { save } = useRuleMutations()

    // The draft's initial value is only meaningful once the record we're
    // editing (if any) has loaded — until then hold off on mounting the form
    // so useRuleDraft doesn't seed itself from a still-loading `null` record.
    if (!recordReady) return <BuilderLoading />
    if (!catalogReady) return <BuilderLoading />
    if (!catalog) return <BuilderCatalogError onRetry={onClose} />

    return (
        <RuleBuilderForm
            // sessionKey (bumped on every open transition, see RuleBuilder
            // below) forces a fresh useRuleDraft seed each time the builder
            // opens — record?.id alone doesn't, because Modal/BottomDrawer
            // content can stay mounted across a close (animation, or a
            // persistently-mounted caller that just toggles isOpen), which
            // would otherwise reopen the same session's stale unsaved draft.
            key={`${sessionKey}:${record?.id ?? 'new'}`}
            onClose={onClose}
            scope={scope}
            presetPkg={presetPkg}
            initial={initialDraft(scope, record)}
            catalog={catalog}
            isLocked={Boolean(ruleId)}
            isSaving={save.isPending}
            onSave={draft => save.mutate(draft, { onSuccess: onClose })}
        />
    )
}

function BuilderLoading() {
    const accent = useThemeColor('primary')
    return (
        <View className="items-center justify-center p-10">
            <ActivityIndicator size="large" color={accent} />
        </View>
    )
}

// The automation catalog is a live query with no explicit error/retry
// primitive exposed by useAutomationCatalog — the practical "retry" here is
// closing so the panel can reopen the builder against a fresh subscription.
function BuilderCatalogError({ onRetry }: { onRetry: () => void }) {
    return (
        <EmptyState
            message="Couldn't load the automation catalog. Try again in a moment."
            action={{ label: 'Close', onPress: onRetry }}
        />
    )
}

interface RuleBuilderFormProps {
    onClose: () => void
    scope: 'personal' | 'org'
    presetPkg?: string
    initial: RuleDraft
    catalog: NonNullable<ReturnType<typeof useAutomationCatalog>['catalog']>
    isLocked: boolean
    isSaving: boolean
    onSave: (draft: RuleDraft) => void
}

function RuleBuilderForm({
    onClose,
    initial,
    catalog,
    isLocked,
    isSaving,
    onSave,
}: RuleBuilderFormProps) {
    const { draft, patch, errors, validate } = useRuleDraft(initial)

    const handleSave = () => {
        if (!validate(catalog)) return
        onSave(draft)
    }

    return (
        <View className="gap-4">
            <BuilderHeader isLocked={isLocked} onClose={onClose} />

            <ScrollView className="max-h-[70vh]">
                <View className="gap-4 pb-2">
                    <PlainInput
                        value={draft.name}
                        onChangeText={name => patch({ name })}
                        placeholder="Rule name"
                        editable={!isLocked}
                        className="text-base font-semibold px-3 py-2 border rounded-lg text-foreground bg-background border-border"
                    />

                    <TriggerCard
                        draft={draft}
                        catalog={catalog}
                        onChange={patch}
                        isLocked={isLocked}
                    />
                    <ConditionsCard draft={draft} catalog={catalog} onChange={patch} />
                    <ActionsCard draft={draft} catalog={catalog} onChange={patch} />
                    <DryRunPanel
                        draft={draft}
                        catalog={catalog}
                        isVisible={Boolean(draft.trigger)}
                    />
                </View>
            </ScrollView>

            <BuilderErrors errors={errors} />

            <BuilderFooter
                draft={draft}
                onChangeStopProcessing={stopProcessing => patch({ stopProcessing })}
                onCancel={onClose}
                onSave={handleSave}
                isSaving={isSaving}
            />
        </View>
    )
}

function BuilderHeader({ isLocked, onClose }: { isLocked: boolean; onClose: () => void }) {
    const mutedColor = useThemeColor('muted-foreground')
    return (
        <View className="flex-row items-center justify-between">
            <Text className="text-lg font-semibold text-foreground">
                {isLocked ? 'Edit rule' : 'New rule'}
            </Text>
            <Button variant="ghost" size="icon" onPress={onClose} accessibilityLabel="Close">
                <X size={18} color={mutedColor} />
            </Button>
        </View>
    )
}

function BuilderErrors({ errors }: { errors: string[] | null }) {
    if (!errors || errors.length === 0) return null
    return (
        <View className="bg-danger-soft border border-danger-soft-foreground rounded-lg p-3">
            <Text className="text-sm font-semibold text-danger mb-2">
                Please fix the following errors:
            </Text>
            {errors.map(error => (
                <Text key={error} className="text-xs text-danger mb-1">
                    {error}
                </Text>
            ))}
        </View>
    )
}

function BuilderFooter({
    draft,
    onChangeStopProcessing,
    onCancel,
    onSave,
    isSaving,
}: {
    draft: RuleDraft
    onChangeStopProcessing: (value: boolean) => void
    onCancel: () => void
    onSave: () => void
    isSaving: boolean
}) {
    return (
        <View className="flex-row items-center justify-between gap-3">
            <View className="flex-row items-center gap-2">
                <Text className="text-sm text-foreground">Stop processing further rules</Text>
                <Switch
                    value={draft.stopProcessing}
                    onValueChange={onChangeStopProcessing}
                    accessibilityLabel="Stop processing further rules"
                />
            </View>
            <View className="flex-row items-center gap-2">
                <Button variant="outline" onPress={onCancel}>
                    <ButtonText>Cancel</ButtonText>
                </Button>
                <Button onPress={onSave} disabled={isSaving}>
                    {isSaving ? <ButtonSpinner /> : null}
                    <ButtonText>Save</ButtonText>
                </Button>
            </View>
        </View>
    )
}

// Responsive wrapper: Modal (size lg) on desktop/tablet web, BottomDrawer on
// mobile — split copied from NotificationDrawer.tsx. Modal already registers
// its own Escape-to-close (scope MODAL, see ui/modal), so nothing extra is
// registered here. BottomDrawer must mount inside the mobile chrome's content
// region to land on the tab bar correctly (see bottom-drawer/index.tsx) — the
// mount screens (Task 8) are responsible for that placement; this component
// only renders the drawer inline where it's used, same as
// NotificationDrawer/FilePickerSheetHost.
export function RuleBuilder({ isOpen, onClose, scope, ruleId, presetPkg }: RuleBuilderProps) {
    const isMobile = useBreakpoint() === 'mobile'

    // Modal/BottomDrawer content can outlive a close (close animation, or a
    // caller — Task 7's panel — that mounts one RuleBuilder instance and just
    // flips isOpen rather than unmounting), so `record?.id` alone isn't
    // enough to key a fresh draft: reopening the same rule (or "new") would
    // reuse the previous session's <RuleBuilderForm>, stale edits and all.
    // Counting open transitions during render (no effect needed — this is a
    // plain derived value, not a side effect) gives BuilderContent a key that
    // changes on every open, guaranteeing useRuleDraft reseeds every time.
    const session = useRef({ wasOpen: false, count: 0 })
    if (isOpen && !session.current.wasOpen) {
        session.current.count += 1
    }
    session.current.wasOpen = isOpen

    if (isMobile) {
        return (
            <BottomDrawer isOpen={isOpen} onClose={onClose}>
                <View className="px-4 pb-4">
                    <BuilderContent
                        onClose={onClose}
                        scope={scope}
                        ruleId={ruleId}
                        presetPkg={presetPkg}
                        sessionKey={session.current.count}
                    />
                </View>
            </BottomDrawer>
        )
    }

    return (
        <Modal isOpen={isOpen} onClose={onClose} size="lg">
            <ModalBackdrop />
            <ModalContent>
                <BuilderContent
                    onClose={onClose}
                    scope={scope}
                    ruleId={ruleId}
                    presetPkg={presetPkg}
                    sessionKey={session.current.count}
                />
            </ModalContent>
        </Modal>
    )
}
