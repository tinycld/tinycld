import type { CatalogResponse } from '@tinycld/core/lib/automation/api'
import { addGroup, setTopMatch } from '@tinycld/core/lib/automation/condition-helpers'
import type { RuleDraft } from '@tinycld/core/lib/automation/draft'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Plus } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { ConditionGroupBox } from './ConditionGroupBox'
import { MatchPillPair } from './MatchPillPair'

export interface ConditionsCardProps {
    draft: RuleDraft
    catalog: CatalogResponse
    onChange: (patch: Partial<RuleDraft>) => void
}

export function ConditionsCard({ draft, catalog, onChange }: ConditionsCardProps) {
    const mutedColor = useThemeColor('muted-foreground')
    const trigger = catalog.triggers.find(t => t.ref === draft.trigger)
    const isVisible = Boolean(trigger && !trigger.synthetic)

    if (!isVisible) return null

    return (
        <View className="rounded-xl border p-4 bg-surface-secondary border-border gap-3">
            <View className="flex-row items-center justify-between">
                <Text className="text-sm font-semibold text-foreground">IF</Text>
                <View className="flex-row items-center gap-2">
                    <Text className="text-xs text-muted-foreground">Match</Text>
                    <MatchPillPair
                        match={draft.conditions.match}
                        onSelect={match =>
                            onChange({ conditions: setTopMatch(draft.conditions, match) })
                        }
                    />
                    <Text className="text-xs text-muted-foreground">groups</Text>
                </View>
            </View>

            {draft.conditions.groups.map((group, groupIndex) => (
                <ConditionGroupBox
                    // Keyed on the builder-local uid (see condition-helpers.ts),
                    // not groupIndex — same reconciliation-identity rationale
                    // as ConditionGroupBox's own ConditionRow keys.
                    key={group.uid}
                    draft={draft}
                    fields={trigger?.fields ?? []}
                    groupIndex={groupIndex}
                    onChange={onChange}
                />
            ))}

            <Pressable
                onPress={() => onChange({ conditions: addGroup(draft.conditions) })}
                className="self-start flex-row items-center gap-1 py-1"
            >
                <Plus size={13} color={mutedColor} />
                <Text className="text-xs text-muted-foreground">add OR group</Text>
            </Pressable>
        </View>
    )
}
