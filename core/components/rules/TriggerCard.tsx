import type { CatalogResponse, CatalogTrigger } from '@tinycld/core/lib/automation/api'
import { orderGroupsByUserPreference } from '@tinycld/core/lib/automation/condition-helpers'
import type { RuleDraft } from '@tinycld/core/lib/automation/draft'
import { FORM_SUBMITTED_TRIGGER } from '@tinycld/core/lib/automation/form-trigger'
import { parseRefSafe } from '@tinycld/core/lib/automation/helpers'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useSortedPackages } from '@tinycld/core/lib/use-sorted-packages'
import { type Control, Controller } from '@tinycld/core/ui/form'
import { Menu } from '@tinycld/core/ui/menu'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { ChevronDown } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { FormPickerRow } from './FormPickerRow'
import { RuleCard } from './RuleCard'

export interface TriggerCardProps {
    control: Control<RuleDraft>
    draft: RuleDraft
    catalog: CatalogResponse
    /** Re-picking the selected trigger is passed on too; the form ignores it. */
    onSelectTrigger: (ref: string) => void
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
        <Menu
            trigger={
                <Pressable className="flex-1 flex-row items-center justify-between border rounded-lg px-2.5 py-1.5 border-border bg-background">
                    <Text className="text-sm text-foreground" numberOfLines={1}>
                        {selected?.label ?? 'Select a trigger…'}
                    </Text>
                    <ChevronDown size={14} color={mutedColor} />
                </Pressable>
            }
            placement="bottom-start"
        >
            {groups.map(([pkg, triggers]) => (
                <Menu.Section key={pkg} label={pkg} testID={`trigger-group-${pkg}`}>
                    {triggers.map(trigger => (
                        <Menu.Item
                            key={trigger.ref}
                            testID={`trigger-option-${trigger.ref}`}
                            label={trigger.label}
                            isSelected={trigger.ref === selectedRef}
                            onSelect={() => onSelect(trigger)}
                        />
                    ))}
                </Menu.Section>
            ))}
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
        <Menu
            trigger={
                <Pressable className="flex-row items-center justify-between border rounded-lg px-2.5 py-1.5 border-border bg-background">
                    <Text className="text-sm text-foreground" numberOfLines={1}>
                        {activePreset?.label ?? 'Custom…'}
                    </Text>
                    <ChevronDown size={14} color={mutedColor} />
                </Pressable>
            }
            placement="bottom-start"
        >
            {SCHEDULE_PRESETS.map(preset => (
                <Menu.Item
                    key={preset.label}
                    label={preset.label}
                    isSelected={preset.cron === cron}
                    onSelect={() => preset.cron && onSelectCron(preset.cron)}
                />
            ))}
        </Menu>
    )
}

function ScheduleRow({ control }: { control: Control<RuleDraft> }) {
    return (
        <Controller
            control={control}
            name="triggerConfig.cron"
            render={({ field }) => (
                <View className="gap-2">
                    <SchedulePresetMenu cron={field.value ?? ''} onSelectCron={field.onChange} />
                    <PlainInput
                        value={field.value ?? ''}
                        onChangeText={field.onChange}
                        onBlur={field.onBlur}
                        placeholder="* * * * *"
                        className="font-mono border rounded-lg px-2.5 py-1.5 text-sm text-foreground bg-background border-border"
                    />
                </View>
            )}
        />
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

export function TriggerCard({
    control,
    draft,
    catalog,
    onSelectTrigger,
    isLocked,
    presetPkg,
}: TriggerCardProps) {
    const isSchedule = draft.trigger === SCHEDULE_TRIGGER_REF
    const triggers = triggersForPreset(catalog, presetPkg)

    return (
        <RuleCard title="WHEN">
            {isLocked ? (
                <LockedTriggerLabel catalog={catalog} triggerRef={draft.trigger} />
            ) : (
                <TriggerMenu
                    catalog={{ ...catalog, triggers }}
                    selectedRef={draft.trigger}
                    onSelect={trigger => onSelectTrigger(trigger.ref)}
                />
            )}
            {isSchedule ? <ScheduleRow control={control} /> : null}
            <FormPickerRow control={control} isVisible={draft.trigger === FORM_SUBMITTED_TRIGGER} />
        </RuleCard>
    )
}
