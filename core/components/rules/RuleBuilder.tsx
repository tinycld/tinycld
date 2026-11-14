import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { EmptyState } from '@tinycld/core/components/EmptyState'
import type { RuleDraft } from '@tinycld/core/lib/automation/draft'
import { emptyDraft, recordToDraft } from '@tinycld/core/lib/automation/draft'
import { extendCatalogForDraft } from '@tinycld/core/lib/automation/form-trigger'
import { useAutomationCatalog } from '@tinycld/core/lib/automation/use-automation-catalog'
import { useRuleDraft } from '@tinycld/core/lib/automation/use-rule-draft'
import { useRuleMutations } from '@tinycld/core/lib/automation/use-rule-mutations'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useFormRow } from '@tinycld/core/lib/forms/use-forms-of-host'
import { useStore } from '@tinycld/core/lib/pocketbase'
import type { BuilderPreset } from '@tinycld/core/lib/stores/rules-ui-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonSpinner, ButtonText } from '@tinycld/core/ui/button'
import { Dialog } from '@tinycld/core/ui/dialog'
import { type Control, Controller, FormErrorSummary } from '@tinycld/core/ui/form'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { Switch } from '@tinycld/core/ui/switch'
import { useEffect, useMemo, useRef } from 'react'
import type { FieldErrors } from 'react-hook-form'
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
    /** Seeds a NEW rule's trigger and config (ignored when editing). */
    preset?: BuilderPreset
}

// Loads the record being edited (if any). Returns `isReady: true` for the
// create case (nothing to load) so the caller doesn't have to special-case
// it, and only actually queries `rules` when a ruleId is present.
function useEditingRecord(ruleId: string | undefined) {
    const [rulesCollection] = useStore('rules')
    const { data, isReady } = useLiveQuery({
        query: query => {
            if (!ruleId) return null
            return query.from({ r: rulesCollection }).where(({ r }) => eq(r.id, ruleId))
        },
    })
    if (!ruleId) return { record: null, isReady: true }
    return { record: data?.[0] ?? null, isReady }
}

// presetPkg only narrows the trigger picker's package group inside
// TriggerCard's menu (see TriggerCard.triggersForPreset) — it isn't a
// trigger ref itself, so it plays no part in seeding the draft. preset does:
// it names the trigger and its config ("Add destination" on a web form).
function initialDraft(
    scope: 'personal' | 'org',
    record: ReturnType<typeof useEditingRecord>['record'],
    nextOrder: number,
    preset?: BuilderPreset
): RuleDraft {
    if (record) return recordToDraft(record)
    const draft = emptyDraft(scope, nextOrder)
    if (!preset) return draft
    return {
        ...draft,
        name: preset.name ?? draft.name,
        trigger: preset.trigger,
        triggerConfig: preset.triggerConfig,
    }
}

function BuilderContent({
    onClose,
    scope,
    ruleId,
    presetPkg,
    nextOrder = 0,
    preset,
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
    // so the form doesn't seed itself from a still-loading `null` record.
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
            initial={initialDraft(scope, record, nextOrder, preset)}
            catalog={catalog}
            isLocked={Boolean(ruleId)}
            isSaving={save.isPending}
            onSave={(draft, onError) => save.mutate(draft, { onSuccess: onClose, onError })}
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
    /** onError puts the server's field errors (a refused rule) on the form. */
    onSave: (draft: RuleDraft, onError: (error: unknown) => void) => void
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
    const form = useRuleDraft(initial, catalog)
    const { control, draft, setFocus } = form
    const formId = draft.triggerConfig.form
    const { row: formRow } = useFormRow(formId)
    // Built after the form row loads in this render, so the schema and the
    // cards never lag one render behind the picked form.
    const draftCatalog = useMemo(
        () =>
            extendCatalogForDraft(
                catalog,
                { trigger: draft.trigger, triggerConfig: { form: formId } },
                formRow?.definition
            ),
        [catalog, draft.trigger, formId, formRow?.definition]
    )
    form.setCatalog(draftCatalog)

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
        const raf = requestAnimationFrame(() => setFocus('name'))
        return () => cancelAnimationFrame(raf)
    }, [shouldFocusName, setFocus])

    const onServerError = handleMutationErrorsWithForm({
        setError: form.setError,
        getValues: form.getValues,
    })
    const handleSave = form.handleSubmit(draft => onSave(draft, onServerError))

    return (
        <>
            <Dialog.Body contentClassName="px-5 pb-3 gap-4">
                <Controller
                    control={control}
                    name="name"
                    render={({ field }) => (
                        <PlainInput
                            ref={field.ref}
                            value={field.value}
                            onChangeText={field.onChange}
                            onBlur={field.onBlur}
                            placeholder="Rule name"
                            autoFocus={shouldFocusName}
                            accessibilityLabel="Rule name"
                            className="text-base font-semibold px-3 py-2 border rounded-lg text-foreground bg-background border-border"
                        />
                    )}
                />

                <TriggerCard
                    control={control}
                    draft={draft}
                    catalog={draftCatalog}
                    onSelectTrigger={form.selectTrigger}
                    isLocked={isLocked}
                    presetPkg={presetPkg}
                />
                <ConditionsCard
                    draft={draft}
                    catalog={draftCatalog}
                    onChangeConditions={form.setConditions}
                />
                <ActionsCard
                    triggerRef={draft.trigger}
                    actionRows={form.actionRows}
                    catalog={draftCatalog}
                    onAdd={form.addAction}
                    onMove={form.moveAction}
                    onRemove={form.removeAction}
                    onChangeParams={form.setActionParams}
                />
                {/* Phase-4 candidate: dry-run resolver-aware scoping (see endpoints.go) */}
                <DryRunPanel
                    draft={draft}
                    catalog={draftCatalog}
                    isVisible={Boolean(draft.trigger)}
                />
            </Dialog.Body>

            <BuilderErrors errors={form.errors} isEnabled={form.isSubmitted} />

            <BuilderFooter
                control={control}
                onCancel={onClose}
                onSave={handleSave}
                isSaving={isSaving}
            />
        </>
    )
}

function BuilderErrors({ errors, isEnabled }: { errors: FieldErrors; isEnabled: boolean }) {
    if (!isEnabled || Object.keys(errors).length === 0) return null
    return (
        // Capped and scrollable: the schema files one error per invalid
        // condition AND per missing action param, so a badly-filled rule can
        // produce a list tall enough to squeeze the scroll region above it to
        // nothing. max-h is intrinsic-until-cap, so the common one- or
        // two-error case still renders at natural height.
        <View className="mx-5">
            <ScrollView className="max-h-[20vh]">
                <FormErrorSummary errors={errors} isEnabled showFieldNames={false} />
            </ScrollView>
        </View>
    )
}

function BuilderFooter({
    control,
    onCancel,
    onSave,
    isSaving,
}: {
    control: Control<RuleDraft>
    onCancel: () => void
    onSave: () => void
    isSaving: boolean
}) {
    return (
        <Dialog.Footer className="justify-between">
            <View className="flex-row items-center gap-2 flex-1">
                <Text className="text-sm text-foreground">Stop processing further rules</Text>
                <Controller
                    control={control}
                    name="stopProcessing"
                    render={({ field }) => (
                        <Switch
                            value={field.value}
                            onValueChange={field.onChange}
                            accessibilityLabel="Stop processing further rules"
                        />
                    )}
                />
            </View>
            <Dialog.CancelButton onPress={onCancel} />
            <Button onPress={onSave} disabled={isSaving} size="sm" testID="rule-builder-save">
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
    preset,
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
                preset={preset}
                sessionKey={session.current.count}
            />
        </Dialog>
    )
}
