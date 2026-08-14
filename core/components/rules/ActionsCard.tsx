import { MenuActionItem } from '@tinycld/core/components/DropdownMenu'
import type {
    CatalogAction,
    CatalogParam,
    CatalogResponse,
    CatalogTrigger,
} from '@tinycld/core/lib/automation/api'
import {
    appendPlaceholder,
    compatibleActions,
    moveAction,
} from '@tinycld/core/lib/automation/condition-helpers'
import type { RuleDraft } from '@tinycld/core/lib/automation/draft'
import { parseRef } from '@tinycld/core/lib/automation/helpers'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import { ArrowDown, ArrowUp, Braces, Plus, Trash2 } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb/core'
import { Fragment } from 'react'
import { Pressable, Text, View } from 'react-native'
import { ValueInput } from './ValueInput'

export interface ActionsCardProps {
    draft: RuleDraft
    catalog: CatalogResponse
    onChange: (patch: Partial<RuleDraft>) => void
}

function groupActionsByPackage(actions: CatalogAction[]): Map<string, CatalogAction[]> {
    const groups = new Map<string, CatalogAction[]>()
    for (const action of actions) {
        const list = groups.get(action.pkg) ?? []
        list.push(action)
        groups.set(action.pkg, list)
    }
    return groups
}

/**
 * The label shown for one action in the add-action menu.
 *
 * Packages pick their own action labels, so collisions are normal — drive and
 * mail both call theirs "Move to folder". The group heading above each item
 * says which package it is, but two identically-worded rows a few lines apart
 * are easy to misread, so an ambiguous label carries its package inline. The
 * unambiguous majority stay clean.
 */
function actionOptionLabel(action: CatalogAction, isAmbiguous: boolean): string {
    if (!action.available) return `${action.label} (needs ${action.pkg})`
    return isAmbiguous ? `${action.label} (${action.pkg})` : action.label
}

/** Labels contributed by more than one package, so they need qualifying. */
function ambiguousLabels(actions: CatalogAction[]): Set<string> {
    const seen = new Map<string, string>()
    const ambiguous = new Set<string>()
    for (const action of actions) {
        const owner = seen.get(action.label)
        if (owner === undefined) {
            seen.set(action.label, action.pkg)
        } else if (owner !== action.pkg) {
            ambiguous.add(action.label)
        }
    }
    return ambiguous
}

function AddActionMenu({
    catalog,
    trigger,
    onSelect,
}: {
    catalog: CatalogResponse
    trigger: CatalogTrigger | undefined
    onSelect: (action: CatalogAction) => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const options = trigger ? compatibleActions(catalog, trigger) : []
    const groups = groupActionsByPackage(options)
    const ambiguous = ambiguousLabels(options)

    return (
        <Menu>
            <Menu.Trigger>
                <Pressable className="self-start flex-row items-center gap-1 py-1">
                    <Plus size={13} color={mutedColor} />
                    <Text className="text-xs text-muted-foreground">add action</Text>
                </Pressable>
            </Menu.Trigger>
            <Menu.Portal>
                <Menu.Overlay />
                <Menu.Content presentation="popover" placement="bottom" align="start">
                    {[...groups.entries()].map(([pkg, actions]) => (
                        <Fragment key={pkg}>
                            <Menu.Label>{pkg}</Menu.Label>
                            {actions.map(action => (
                                <MenuActionItem
                                    key={action.ref}
                                    testID={`action-option-${action.ref}`}
                                    label={actionOptionLabel(action, ambiguous.has(action.label))}
                                    disabled={!action.available}
                                    onPress={() => onSelect(action)}
                                />
                            ))}
                        </Fragment>
                    ))}
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}

function TemplateFieldMenu({
    trigger,
    onSelectKey,
}: {
    trigger: CatalogTrigger | undefined
    onSelectKey: (key: string) => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const fields = trigger?.fields ?? []
    if (fields.length === 0) return null

    return (
        <Menu>
            <Menu.Trigger>
                <Pressable className="p-1.5" hitSlop={8}>
                    <Braces size={14} color={mutedColor} />
                </Pressable>
            </Menu.Trigger>
            <Menu.Portal>
                <Menu.Overlay />
                <Menu.Content presentation="popover" placement="bottom" align="start">
                    {fields.map(field => (
                        <MenuActionItem
                            key={field.key}
                            label={field.label}
                            onPress={() => onSelectKey(field.key)}
                        />
                    ))}
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}

function ActionParamRow({
    param,
    trigger,
    value,
    onChange,
}: {
    param: CatalogParam
    trigger: CatalogTrigger | undefined
    value: string | number | boolean | undefined
    onChange: (v: string | number | boolean) => void
}) {
    const isTemplateText = param.template && param.field.type === 'text'
    const textValue = typeof value === 'string' ? value : ''

    return (
        <View className="gap-1">
            <Text className="text-xs text-muted-foreground">{param.label}</Text>
            <View className="flex-row items-center gap-1">
                <ValueInput field={param.field} op="eq" value={value} onChange={onChange} />
                {isTemplateText ? (
                    <TemplateFieldMenu
                        trigger={trigger}
                        onSelectKey={key => onChange(appendPlaceholder(textValue, key))}
                    />
                ) : null}
            </View>
        </View>
    )
}

function ActionEntry({
    index,
    total,
    draftAction,
    catalog,
    trigger,
    onChangeParams,
    onMove,
    onRemove,
}: {
    index: number
    total: number
    draftAction: RuleDraft['actions'][number]
    catalog: CatalogResponse
    trigger: CatalogTrigger | undefined
    onChangeParams: (params: Record<string, string | number | boolean>) => void
    onMove: (to: number) => void
    onRemove: () => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const action = catalog.actions.find(a => a.ref === draftAction.ref)
    const { pkg, id } = parseRef(draftAction.ref)

    const handleParamChange = (key: string, value: string | number | boolean) => {
        onChangeParams({ ...draftAction.params, [key]: value })
    }

    return (
        <View className="rounded-lg border p-3 gap-2.5 border-border">
            <View className="flex-row items-center justify-between">
                <Text className="text-sm text-foreground">
                    {index + 1}. {action?.label ?? id}
                    <Text className="text-xs text-muted-foreground"> ({pkg})</Text>
                </Text>
                <View className="flex-row items-center gap-1">
                    <Pressable
                        onPress={() => onMove(index - 1)}
                        disabled={index === 0}
                        className={`p-1.5 ${index === 0 ? 'opacity-40' : 'opacity-100'}`}
                        hitSlop={8}
                    >
                        <ArrowUp size={14} color={mutedColor} />
                    </Pressable>
                    <Pressable
                        onPress={() => onMove(index + 1)}
                        disabled={index === total - 1}
                        className={`p-1.5 ${index === total - 1 ? 'opacity-40' : 'opacity-100'}`}
                        hitSlop={8}
                    >
                        <ArrowDown size={14} color={mutedColor} />
                    </Pressable>
                    <Pressable onPress={onRemove} className="p-1.5" hitSlop={8}>
                        <Trash2 size={14} color={mutedColor} />
                    </Pressable>
                </View>
            </View>

            {(action?.params ?? []).map(param => (
                <ActionParamRow
                    key={param.key}
                    param={param}
                    trigger={trigger}
                    value={draftAction.params[param.key]}
                    onChange={v => handleParamChange(param.key, v)}
                />
            ))}
        </View>
    )
}

export function ActionsCard({ draft, catalog, onChange }: ActionsCardProps) {
    const trigger = catalog.triggers.find(t => t.ref === draft.trigger)

    const handleSelectAction = (action: CatalogAction) => {
        onChange({
            actions: [...draft.actions, { uid: newRecordId(), ref: action.ref, params: {} }],
        })
    }

    const handleChangeParams = (
        index: number,
        params: Record<string, string | number | boolean>
    ) => {
        onChange({
            actions: draft.actions.map((a, i) => (i === index ? { ...a, params } : a)),
        })
    }

    const handleMove = (from: number, to: number) => {
        onChange({ actions: moveAction(draft.actions, from, to) })
    }

    const handleRemove = (index: number) => {
        onChange({ actions: draft.actions.filter((_, i) => i !== index) })
    }

    return (
        <View className="rounded-xl border p-4 bg-surface-secondary border-border gap-3">
            <Text className="text-sm font-semibold text-foreground">THEN</Text>

            {draft.actions.map((draftAction, index) => (
                <ActionEntry
                    // Keyed on the builder-local uid (condition-helpers.ts's
                    // ensureActionUids/ActionsCard's handleSelectAction), not
                    // index — moveAction actively remaps index on every up/down
                    // reorder, and an index key would reconcile an open param
                    // Menu (TemplateFieldMenu/ValueInput's relation & select
                    // Menus own isOpen internally) or a focused input onto
                    // whichever action slides into that slot. Same rationale as
                    // ConditionRow/ConditionGroupBox's uid keys.
                    key={draftAction.uid}
                    index={index}
                    total={draft.actions.length}
                    draftAction={draftAction}
                    catalog={catalog}
                    trigger={trigger}
                    onChangeParams={params => handleChangeParams(index, params)}
                    onMove={to => handleMove(index, to)}
                    onRemove={() => handleRemove(index)}
                />
            ))}

            <AddActionMenu catalog={catalog} trigger={trigger} onSelect={handleSelectAction} />
        </View>
    )
}
