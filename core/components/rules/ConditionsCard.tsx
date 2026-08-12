import type { CatalogField, CatalogResponse } from '@tinycld/core/lib/automation/api'
import {
    addCondition,
    addGroup,
    removeCondition,
    removeGroup,
    setGroupMatch,
    setTopMatch,
    updateCondition,
} from '@tinycld/core/lib/automation/condition-helpers'
import type { RuleDraft } from '@tinycld/core/lib/automation/draft'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Plus, Trash2 } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { ConditionRow } from './ConditionRow'

export interface ConditionsCardProps {
    draft: RuleDraft
    catalog: CatalogResponse
    onChange: (patch: Partial<RuleDraft>) => void
}

function MatchPill({
    label,
    isActive,
    onPress,
}: {
    label: string
    isActive: boolean
    onPress: () => void
}) {
    return (
        <Pressable
            onPress={onPress}
            className={`px-2.5 py-1 rounded-md ${isActive ? 'bg-primary' : 'border border-border'}`}
        >
            <Text className={`text-xs ${isActive ? 'text-primary-foreground' : 'text-primary'}`}>
                {label}
            </Text>
        </Pressable>
    )
}

function MatchPillPair({
    match,
    onSelect,
}: {
    match: 'all' | 'any'
    onSelect: (match: 'all' | 'any') => void
}) {
    return (
        <View className="flex-row gap-1.5">
            <MatchPill label="all" isActive={match === 'all'} onPress={() => onSelect('all')} />
            <MatchPill label="any" isActive={match === 'any'} onPress={() => onSelect('any')} />
        </View>
    )
}

function GroupBox({
    draft,
    fields,
    groupIndex,
    onChange,
}: {
    draft: RuleDraft
    fields: CatalogField[]
    groupIndex: number
    onChange: (patch: Partial<RuleDraft>) => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const group = draft.conditions.groups[groupIndex]

    return (
        <View className="rounded-lg border p-3 gap-2.5 border-border">
            <View className="flex-row items-center justify-between">
                <View className="flex-row items-center gap-2">
                    <Text className="text-xs text-muted-foreground">Match</Text>
                    <MatchPillPair
                        match={group.match}
                        onSelect={match =>
                            onChange({
                                conditions: setGroupMatch(draft.conditions, groupIndex, match),
                            })
                        }
                    />
                    <Text className="text-xs text-muted-foreground">conditions</Text>
                </View>
                <Pressable
                    onPress={() =>
                        onChange({ conditions: removeGroup(draft.conditions, groupIndex) })
                    }
                    className="p-1.5"
                    hitSlop={8}
                >
                    <Trash2 size={14} color={mutedColor} />
                </Pressable>
            </View>

            {group.conditions.map((condition, conditionIndex) => (
                <ConditionRow
                    key={conditionIndex}
                    condition={condition}
                    fields={fields}
                    onChange={patch =>
                        onChange({
                            conditions: updateCondition(
                                draft.conditions,
                                groupIndex,
                                conditionIndex,
                                patch
                            ),
                        })
                    }
                    onRemove={() =>
                        onChange({
                            conditions: removeCondition(
                                draft.conditions,
                                groupIndex,
                                conditionIndex
                            ),
                        })
                    }
                />
            ))}

            <Pressable
                onPress={() => onChange({ conditions: addCondition(draft.conditions, groupIndex) })}
                className="self-start flex-row items-center gap-1 py-1"
            >
                <Plus size={13} color={mutedColor} />
                <Text className="text-xs text-muted-foreground">add condition</Text>
            </Pressable>
        </View>
    )
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

            {draft.conditions.groups.map((_group, groupIndex) => (
                <GroupBox
                    key={groupIndex}
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
