import { MenuActionItem } from '@tinycld/core/components/DropdownMenu'
import type { CatalogResponse, CatalogTrigger } from '@tinycld/core/lib/automation/api'
import { orderGroupsByUserPreference } from '@tinycld/core/lib/automation/condition-helpers'
import type { RuleDraft } from '@tinycld/core/lib/automation/draft'
import { parseRefSafe } from '@tinycld/core/lib/automation/helpers'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useSortedPackages } from '@tinycld/core/lib/use-sorted-packages'
import { Menu } from '@tinycld/core/ui/menu'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { ChevronDown } from 'lucide-react-native'
import { Fragment } from 'react'
import { Pressable, Text, View } from 'react-native'
import { RuleCard } from './RuleCard'

export interface TriggerCardProps {
    draft: RuleDraft
    catalog: CatalogResponse
    onChange: (patch: Partial<RuleDraft>) => void
    isLocked: boolean
    /** Narrows the trigger picker to one package's triggers (e.g. mail's
     * "Rules" screen only offers mail-flavored automation) — core's
     * synthetic triggers (manual/schedule) stay available regardless, since
     * they're package-neutral starting points, not mail/calendar/etc-owned. */
    presetPkg?: string
}

const SCHEDULE_TRIGGER_REF = 'core:schedule'

const SCHEDULE_PRESETS = [
    { label: 'Every hour', cron: '0 * * * *' },
    { label: 'Every day at 8:00', cron: '0 8 * * *' },
    { label: 'Every Monday at 8:00', cron: '0 8 * * 1' },
    { label: 'Custom…', cron: null },
] as const

function groupTriggersByPackage(triggers: CatalogTrigger[]): Map<string, CatalogTrigger[]> {
    const groups = new Map<string, CatalogTrigger[]>()
    for (const trigger of triggers) {
        const list = groups.get(trigger.pkg) ?? []
        list.push(trigger)
        groups.set(trigger.pkg, list)
    }
    return groups
}

function TriggerMenu({
    catalog,
    selectedRef,
    onSelect,
}: {
    catalog: CatalogResponse
    selectedRef: string
    onSelect: (trigger: CatalogTrigger) => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const selected = catalog.triggers.find(t => t.ref === selectedRef)
    // Same order the user dragged their apps into, so the menu reads like the
    // sidebar. core's package-neutral triggers always lead.
    const orderedSlugs = useSortedPackages().map(p => p.slug)
    const groups = orderGroupsByUserPreference(
        groupTriggersByPackage(catalog.triggers),
        orderedSlugs
    )

    return (
        <Menu>
            <Menu.Trigger>
                <Pressable className="flex-1 flex-row items-center justify-between border rounded-lg px-2.5 py-1.5 border-border bg-background">
                    <Text className="text-sm text-foreground" numberOfLines={1}>
                        {selected?.label ?? 'Select a trigger…'}
                    </Text>
                    <ChevronDown size={14} color={mutedColor} />
                </Pressable>
            </Menu.Trigger>
            <Menu.Portal>
                <Menu.Overlay />
                <Menu.Content presentation="popover" placement="bottom" align="start">
                    {groups.map(([pkg, triggers]) => (
                        <Fragment key={pkg}>
                            <Menu.Label testID={`trigger-group-${pkg}`}>{pkg}</Menu.Label>
                            {triggers.map(trigger => (
                                <MenuActionItem
                                    key={trigger.ref}
                                    testID={`trigger-option-${trigger.ref}`}
                                    label={trigger.label}
                                    isActive={trigger.ref === selectedRef}
                                    onPress={() => onSelect(trigger)}
                                />
                            ))}
                        </Fragment>
                    ))}
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}

function LockedTriggerLabel({
    triggerRef,
    catalog,
}: {
    triggerRef: string
    catalog: CatalogResponse
}) {
    const trigger = catalog.triggers.find(t => t.ref === triggerRef)
    const { pkg } = parseRefSafe(triggerRef)
    return (
        <Text className="text-sm text-foreground">
            {trigger?.label ?? triggerRef}
            <Text className="text-xs text-muted-foreground"> ({pkg})</Text>
        </Text>
    )
}

function SchedulePresetMenu({
    cron,
    onSelectCron,
}: {
    cron: string
    onSelectCron: (cron: string) => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const activePreset = SCHEDULE_PRESETS.find(p => p.cron === cron)

    return (
        <Menu>
            <Menu.Trigger>
                <Pressable className="flex-row items-center justify-between border rounded-lg px-2.5 py-1.5 border-border bg-background">
                    <Text className="text-sm text-foreground" numberOfLines={1}>
                        {activePreset?.label ?? 'Custom…'}
                    </Text>
                    <ChevronDown size={14} color={mutedColor} />
                </Pressable>
            </Menu.Trigger>
            <Menu.Portal>
                <Menu.Overlay />
                <Menu.Content presentation="popover" placement="bottom" align="start">
                    {SCHEDULE_PRESETS.map(preset => (
                        <MenuActionItem
                            key={preset.label}
                            label={preset.label}
                            isActive={preset.cron === cron}
                            onPress={() => preset.cron && onSelectCron(preset.cron)}
                        />
                    ))}
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}

function ScheduleRow({
    draft,
    onChange,
}: {
    draft: RuleDraft
    onChange: (patch: Partial<RuleDraft>) => void
}) {
    const cron = draft.triggerConfig.cron ?? ''

    const handleCronText = (text: string) => {
        onChange({ triggerConfig: { ...draft.triggerConfig, cron: text } })
    }

    return (
        <View className="gap-2">
            <SchedulePresetMenu cron={cron} onSelectCron={handleCronText} />
            <PlainInput
                value={cron}
                onChangeText={handleCronText}
                placeholder="* * * * *"
                className="font-mono border rounded-lg px-2.5 py-1.5 text-sm text-foreground bg-background border-border"
            />
        </View>
    )
}

// presetPkg narrows the picker to that package's own triggers, plus core's
// synthetic built-ins (manual/schedule) — those are scope-neutral starting
// points, not owned by any one feature package, so they stay visible in a
// preset view. Only the trigger LIST is filtered; catalog.actions is passed
// through unfiltered everywhere else since cross-package actions (e.g. mail
// rules notifying via a core action) are the point.
function triggersForPreset(catalog: CatalogResponse, presetPkg: string | undefined) {
    if (!presetPkg) return catalog.triggers
    return catalog.triggers.filter(t => t.pkg === presetPkg || t.synthetic)
}

export function TriggerCard({ draft, catalog, onChange, isLocked, presetPkg }: TriggerCardProps) {
    const isSchedule = draft.trigger === SCHEDULE_TRIGGER_REF
    const triggers = triggersForPreset(catalog, presetPkg)

    const handleSelectTrigger = (trigger: CatalogTrigger) => {
        // Re-picking the trigger already selected is a no-op, not a reset:
        // the menu shows the current choice, so clicking it is the natural way
        // to close the menu — and it used to silently discard every condition
        // and action the user had built.
        if (trigger.ref === draft.trigger) return
        // Switching triggers invalidates conditions (built against the old
        // trigger's field set) and actions (may target the old trigger's
        // collection) — both reset rather than silently carrying over.
        //
        // Resetting to zero groups is also what re-arms ConditionsCard's ready
        // first row against the NEW trigger's fields (see its SyntheticFirstGroup).
        // The literal deliberately skips ensureUids, which is safe only because
        // the array is empty — anything seeded here would need uids to key on.
        onChange({
            trigger: trigger.ref,
            conditions: { match: 'all', groups: [] },
            actions: [],
        })
    }

    return (
        <RuleCard title="WHEN">
            {isLocked ? (
                <LockedTriggerLabel catalog={catalog} triggerRef={draft.trigger} />
            ) : (
                <TriggerMenu
                    catalog={{ ...catalog, triggers }}
                    selectedRef={draft.trigger}
                    onSelect={handleSelectTrigger}
                />
            )}
            {isSchedule ? <ScheduleRow draft={draft} onChange={onChange} /> : null}
        </RuleCard>
    )
}
