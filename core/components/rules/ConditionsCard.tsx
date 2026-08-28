import type { CatalogField, CatalogResponse } from '@tinycld/core/lib/automation/api'
import type { Condition } from '@tinycld/core/lib/automation/condition-helpers'
import {
    addCondition,
    addGroup,
    seedFirstCondition,
    setTopMatch,
} from '@tinycld/core/lib/automation/condition-helpers'
import type { RuleDraft } from '@tinycld/core/lib/automation/draft'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Plus } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { ConditionGroupBox } from './ConditionGroupBox'
import { ConditionRow } from './ConditionRow'
import { MatchPillPair } from './MatchPillPair'
import { RuleCard } from './RuleCard'

export interface ConditionsCardProps {
    draft: RuleDraft
    catalog: CatalogResponse
    onChange: (patch: Partial<RuleDraft>) => void
}

const NO_TRIGGER_HINT = 'Choose a trigger first'
// Distinct from NO_TRIGGER_HINT on purpose: "choose a trigger" is something the
// user can act on, whereas a schedule/manual rule has no record behind it and
// so can never have conditions. One string for both would be wrong in one case.
const SYNTHETIC_HINT = 'This trigger has no fields to filter on'

/**
 * The ready-to-fill first condition, shown when the draft has no groups yet.
 *
 * Rendered, NOT seeded into the draft. Reaching a first condition used to take
 * two clicks through "add OR group" — a control that means nothing when there
 * is nothing to OR with — and seeding a blank row into the draft instead would
 * break saving outright: the zod schemas require a non-empty field and at least
 * one condition per group, so a blank row throws in draftToRecord. Keeping it
 * render-only means an untouched builder still saves a rule with no conditions.
 */
function SyntheticFirstGroup({
    isVisible,
    fields,
    onSeed,
}: {
    isVisible: boolean
    fields: CatalogField[]
    onSeed: (patch: Partial<Condition>) => void
}) {
    if (!isVisible) return null
    return (
        <View className="rounded-lg border p-3 gap-2.5 border-border">
            <ConditionRow
                condition={{ field: '', op: '', value: undefined }}
                fields={fields}
                onChange={onSeed}
            />
        </View>
    )
}

export function ConditionsCard({ draft, catalog, onChange }: ConditionsCardProps) {
    const mutedColor = useThemeColor('muted-foreground')
    const trigger = catalog.triggers.find(t => t.ref === draft.trigger)
    const isSynthetic = Boolean(trigger?.synthetic)
    const isDisabled = !trigger || isSynthetic
    const hasGroups = draft.conditions.groups.length > 0

    const handleSeed = (patch: Partial<Condition>) =>
        onChange({ conditions: seedFirstCondition(draft.conditions, patch) })

    // Adding the FIRST group has to carry a condition row with it. The offered
    // row above is render-only and disappears the moment a real group exists,
    // so appending an empty group would make the row the user was looking at
    // vanish and be replaced by a group with nothing in it — the work they were
    // about to do, silently discarded. Later groups start empty as before.
    const handleAddGroup = () =>
        onChange({
            conditions: hasGroups
                ? addGroup(draft.conditions)
                : addCondition(addGroup(draft.conditions), 0),
        })

    const matchPills = (
        <View className="flex-row items-center gap-2">
            <Text className="text-xs text-muted-foreground">Match</Text>
            <MatchPillPair
                match={draft.conditions.match}
                onSelect={match => onChange({ conditions: setTopMatch(draft.conditions, match) })}
            />
            <Text className="text-xs text-muted-foreground">groups</Text>
        </View>
    )

    return (
        <RuleCard
            title="IF"
            isDisabled={isDisabled}
            disabledHint={trigger ? SYNTHETIC_HINT : NO_TRIGGER_HINT}
            trailing={matchPills}
        >
            <SyntheticFirstGroup
                isVisible={!isDisabled && !hasGroups}
                fields={trigger?.fields ?? []}
                onSeed={handleSeed}
            />

            {draft.conditions.groups.map((group, groupIndex) => (
                <ConditionGroupBox
                    // Keyed on the builder-local uid (see condition-helpers.ts),
                    // not groupIndex — same reconciliation-identity rationale
                    // as ConditionGroupBox's own ConditionRow keys.
                    key={group.uid}
                    draft={draft}
                    fields={trigger?.fields ?? []}
                    group={group}
                    groupIndex={groupIndex}
                    onChange={onChange}
                />
            ))}

            {/* Secondary to the "add condition" link inside each group: adding a
                whole OR group is the rarer, more advanced move, and leading with
                it is what made the first condition hard to find. */}
            <Pressable
                onPress={handleAddGroup}
                className="self-start flex-row items-center gap-1 py-1 opacity-70"
            >
                <Plus size={12} color={mutedColor} />
                <Text className="text-[11px] text-muted-foreground">add OR group</Text>
            </Pressable>
        </RuleCard>
    )
}
