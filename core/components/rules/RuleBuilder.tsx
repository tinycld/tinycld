import { eq } from '@tanstack/db'
import { EmptyState } from '@tinycld/core/components/EmptyState'
import type { RuleDraft } from '@tinycld/core/lib/automation/draft'
import { emptyDraft, recordToDraft } from '@tinycld/core/lib/automation/draft'
import { useAutomationCatalog } from '@tinycld/core/lib/automation/use-automation-catalog'
import { useRuleDraft } from '@tinycld/core/lib/automation/use-rule-draft'
import { useRuleMutations } from '@tinycld/core/lib/automation/use-rule-mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { Button, ButtonSpinner, ButtonText } from '@tinycld/core/ui/button'
import { Dialog } from '@tinycld/core/ui/dialog'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { Switch } from '@tinycld/core/ui/switch'
import { useEffect, useRef } from 'react'
import { ActivityIndicator, ScrollView, Text, type TextInput, View } from 'react-native'
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
}: Omit<RuleBuilderProps, 'isOpen'> & { sessionKey: number }) {
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
            // opens — record?.id alone does not, because Dialog
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
    presetPkg,
    initial,
    catalog,
    isLocked,
    isSaving,
    onSave,
}: RuleBuilderFormProps) {
    const { draft, patch, errors, validate } = useRuleDraft(initial)
    const nameRef = useRef<TextInput>(null)

    // Naming the rule is the first thing every new rule needs, so start there
    // rather than making the user click in.
    //
    // Only for a NEW rule: opening an existing rule is usually to change a
    // condition or an action, so grabbing focus would fight what the user
    // came to do.
    const shouldFocusName = !isLocked

    // After the dialog's own focus pass, which runs on mount and would
    // otherwise land on the first control instead.
    useEffect(() => {
        if (!shouldFocusName) return
        const raf = requestAnimationFrame(() => nameRef.current?.focus())
        return () => cancelAnimationFrame(raf)
    }, [shouldFocusName])

    const handleSave = () => {
        if (!validate(catalog)) return
        onSave(draft)
    }

    return (
        <>
            <Dialog.Body contentClassName="px-5 pb-3 gap-4">
                <PlainInput
                    ref={nameRef}
                    value={draft.name}
                    onChangeText={name => patch({ name })}
                    placeholder="Rule name"
                    autoFocus={shouldFocusName}
                    accessibilityLabel="Rule name"
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
                <DryRunPanel draft={draft} catalog={catalog} isVisible={Boolean(draft.trigger)} />
            </Dialog.Body>

            <BuilderErrors errors={errors} />

            <BuilderFooter
                draft={draft}
                onChangeStopProcessing={stopProcessing => patch({ stopProcessing })}
                onCancel={onClose}
                onSave={handleSave}
                isSaving={isSaving}
            />
        </>
    )
}

function BuilderErrors({ errors }: { errors: string[] | null }) {
    if (!errors || errors.length === 0) return null
    return (
        <View className="mx-5 mb-3 bg-danger-soft border border-danger-soft-foreground rounded-lg p-3">
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
        <Dialog.Footer className="justify-between">
            <View className="flex-row items-center gap-2 flex-1">
                <Text className="text-sm text-foreground">Stop processing further rules</Text>
                <Switch
                    value={draft.stopProcessing}
                    onValueChange={onChangeStopProcessing}
                    accessibilityLabel="Stop processing further rules"
                />
            </View>
            <Dialog.CancelButton onPress={onCancel} />
            <Button onPress={onSave} disabled={isSaving} size="sm">
                {isSaving ? <ButtonSpinner /> : null}
                <ButtonText>Save</ButtonText>
            </Button>
        </Dialog.Footer>
    )
}

// A Dialog on the desktop layout and a sheet on a phone, which the Dialog
// decides for itself. Escape is the dialog's own.
export function RuleBuilder({
    isOpen,
    onClose,
    scope,
    ruleId,
    presetPkg,
    nextOrder,
}: RuleBuilderProps) {
    // Dialog content can outlive a close (close animation, or a
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

    return (
        <Dialog
            isOpen={isOpen}
            onClose={onClose}
            title={ruleId ? 'Edit rule' : 'New rule'}
            size="lg"
        >
            <BuilderContent
                onClose={onClose}
                scope={scope}
                ruleId={ruleId}
                presetPkg={presetPkg}
                nextOrder={nextOrder}
                sessionKey={session.current.count}
            />
        </Dialog>
    )
}
