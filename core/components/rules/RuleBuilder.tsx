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
import { useEffect, useRef } from 'react'
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
    /** Order to seed a NEW rule's draft with (ignored when editing — the
     * loaded record's own order wins). Callers compute max(existing order) +
     * 1 so ties can't cause display/execution divergence; see RulesPanel. */
    nextOrder?: number
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

// presetPkg only narrows the trigger picker's package group inside
// TriggerCard's menu (see TriggerCard.triggersForPreset) — it isn't a
// trigger ref itself, so it plays no part in seeding the draft.
function initialDraft(
    scope: 'personal' | 'org',
    record: ReturnType<typeof useEditingRecord>['record'],
    nextOrder: number
): RuleDraft {
    return record ? recordToDraft(record) : emptyDraft(scope, nextOrder)
}

function BuilderContent({
    onClose,
    scope,
    ruleId,
    presetPkg,
    nextOrder = 0,
    sessionKey,
    isMobile,
}: Omit<RuleBuilderProps, 'isOpen'> & { sessionKey: number; isMobile: boolean }) {
    const { record, isReady: recordReady } = useEditingRecord(ruleId)
    const { catalog, isReady: catalogReady } = useAutomationCatalog()
    const { save } = useRuleMutations()

    // ruleId set but the record vanished (deleted while the builder was
    // open, or a stale/bad id) — closing avoids showing an empty "Edit rule"
    // form whose Save would silently CREATE a new rule instead of updating
    // the one the user thought they were editing. A genuine side effect
    // (calling the parent's close callback), not a derivable render value,
    // so it belongs in an effect rather than being invoked during render.
    const recordVanished = recordReady && Boolean(ruleId) && !record
    useEffect(() => {
        if (recordVanished) onClose()
    }, [recordVanished, onClose])

    // The draft's initial value is only meaningful once the record we're
    // editing (if any) has loaded — until then hold off on mounting the form
    // so useRuleDraft doesn't seed itself from a still-loading `null` record.
    if (!recordReady) return <BuilderLoading />
    if (recordVanished) return <BuilderLoading />
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
            initial={initialDraft(scope, record, nextOrder)}
            catalog={catalog}
            isLocked={Boolean(ruleId)}
            isSaving={save.isPending}
            isMobile={isMobile}
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
    isMobile: boolean
    onSave: (draft: RuleDraft) => void
}

// The scroll region's sizing differs by shell, because the two shells bound
// their content differently.
//
// Modal: ModalContent is `overflow-hidden` with no height of its own, so the
// form caps it (max-h-[85vh] below) and the middle band takes whatever is left
// via flex — `min-h-0` is required, since RN's `min-height: auto` default
// otherwise refuses to shrink a flex child below its content and the cap is
// ignored, clipping the footer off-screen.
//
// BottomDrawer: the sheet measures its own height from its content (onLayout)
// and caps at 85% of the screen. A `flex-1` child inside an intrinsically-sized
// parent has no definite height to flex against and can collapse to zero, so
// the drawer path keeps an explicit cap instead. 70vh sits under the sheet's
// own 85% so the header and footer stay inside the sheet.
function scrollRegionClass(isMobile: boolean): string {
    return isMobile ? 'max-h-[70vh]' : 'flex-1 min-h-0'
}

function RuleBuilderForm({
    onClose,
    presetPkg,
    initial,
    catalog,
    isLocked,
    isSaving,
    isMobile,
    onSave,
}: RuleBuilderFormProps) {
    const { draft, patch, errors, validate } = useRuleDraft(initial)

    const handleSave = () => {
        if (!validate(catalog)) return
        onSave(draft)
    }

    return (
        <View className={isMobile ? 'gap-4' : 'gap-4 flex-1 min-h-0'}>
            <BuilderHeader isLocked={isLocked} onClose={onClose} />

            <ScrollView className={scrollRegionClass(isMobile)}>
                <View className="gap-4 pb-2">
                    <PlainInput
                        value={draft.name}
                        onChangeText={name => patch({ name })}
                        placeholder="Rule name"
                        className="text-base font-semibold px-3 py-2 border rounded-lg text-foreground bg-background border-border"
                    />

                    <TriggerCard
                        draft={draft}
                        catalog={catalog}
                        onChange={patch}
                        isLocked={isLocked}
                        presetPkg={presetPkg}
                    />
                    <ConditionsCard draft={draft} catalog={catalog} onChange={patch} />
                    <ActionsCard draft={draft} catalog={catalog} onChange={patch} />
                    {/* Phase-4 candidate: dry-run resolver-aware scoping (see endpoints.go) */}
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
            {/* Capped and scrollable: validateDraft emits one error per invalid
                condition AND per missing action param, so a badly-filled rule can
                produce a list tall enough to squeeze the scroll region above it to
                nothing. max-h is intrinsic-until-cap, so the common one- or
                two-error case still renders at natural height. The heading stays
                outside so it is never scrolled out of view. */}
            <ScrollView className="max-h-[20vh]">
                {errors.map(error => (
                    <Text key={error} className="text-xs text-danger mb-1">
                        {error}
                    </Text>
                ))}
            </ScrollView>
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
export function RuleBuilder({
    isOpen,
    onClose,
    scope,
    ruleId,
    presetPkg,
    nextOrder,
}: RuleBuilderProps) {
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
                        nextOrder={nextOrder}
                        sessionKey={session.current.count}
                        isMobile
                    />
                </View>
            </BottomDrawer>
        )
    }

    return (
        <Modal isOpen={isOpen} onClose={onClose} size="lg">
            <ModalBackdrop />
            {/* ModalContent is `overflow-hidden` with no height of its own, so
                without this cap the form's header + scroll region + errors +
                footer can exceed the viewport and the excess is CLIPPED — and
                because modalStyle centers the content, what gets clipped is the
                footer, stranding Save and Cancel off-screen. 85vh matches
                BottomDrawer's own 85% cap. */}
            <ModalContent className="max-h-[85vh]">
                <BuilderContent
                    onClose={onClose}
                    scope={scope}
                    ruleId={ruleId}
                    presetPkg={presetPkg}
                    nextOrder={nextOrder}
                    sessionKey={session.current.count}
                    isMobile={false}
                />
            </ModalContent>
        </Modal>
    )
}
