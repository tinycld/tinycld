// One row in RulesPanel: drag handle · name/summary · badges · last-run line
// · enabled Switch · overflow Menu (Edit / Run history / Run now / Delete).

import { DotsMenu, MenuActionItem, MenuSeparator } from '@tinycld/core/components/DropdownMenu'
import { formatRelativeTime } from '@tinycld/core/components/NotificationDrawer'
import { needsPackage, ruleSummary } from '@tinycld/core/components/rules/rule-summary'
import { SortableDragHandle } from '@tinycld/core/components/SortableList'
import type { CatalogResponse } from '@tinycld/core/lib/automation/api'
import { useRuleMutations } from '@tinycld/core/lib/automation/use-rule-mutations'
import { useRulesUiStore } from '@tinycld/core/lib/stores/rules-ui-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import type { Rules } from '@tinycld/core/types/pbSchema'
import { ConfirmDialog } from '@tinycld/core/ui/ConfirmDialog'
import { Switch } from '@tinycld/core/ui/switch'
import { Clock, History, Pencil, Play, Trash2 } from 'lucide-react-native'
import { useState } from 'react'
import { Text, View } from 'react-native'

export interface RuleRowProps {
    rule: Rules
    catalog: CatalogResponse
    canEdit: boolean
    /** Newest run for this rule, if any — resolved once at the panel level. */
    lastRun?: { fired_at: string; matched: boolean }
    /** Show an "Organization" pill — set by the panel only in mixed contexts. */
    showScopeBadge?: boolean
}

// synthetic-trigger rules (manual/schedule) are the only ones "Run now" makes
// sense for — everything else fires off a real collection event the engine
// can't synthesize on demand.
function isSyntheticTrigger(rule: Rules, catalog: CatalogResponse): boolean {
    return Boolean(catalog.triggers.find(t => t.ref === rule.trigger)?.synthetic)
}

export function RuleRow({ rule, catalog, canEdit, lastRun, showScopeBadge }: RuleRowProps) {
    const [isConfirmingDelete, setIsConfirmingDelete] = useState(false)
    const { setEnabled, remove } = useRuleMutations()
    const openEdit = useRulesUiStore(s => s.openEdit)
    const openHistory = useRulesUiStore(s => s.openHistory)

    const missingPkg = needsPackage(rule, catalog)
    const summary = ruleSummary(rule, catalog)

    return (
        <View
            className="flex-row items-center gap-3 px-3 py-3 border-b border-border"
            style={{ opacity: missingPkg ? 0.6 : 1 }}
        >
            <SortableDragHandle disabled={!canEdit} />

            <RuleRowMain
                rule={rule}
                summary={summary}
                missingPkg={missingPkg}
                showScopeBadge={Boolean(showScopeBadge)}
                lastRun={lastRun}
            />

            <Switch
                value={rule.enabled}
                onValueChange={enabled => setEnabled.mutate({ id: rule.id, enabled })}
                disabled={!canEdit}
                accessibilityLabel={`${rule.enabled ? 'Disable' : 'Enable'} ${rule.name}`}
            />

            <RuleRowMenu
                rule={rule}
                canEdit={canEdit}
                canRunNow={isSyntheticTrigger(rule, catalog)}
                onEdit={() => openEdit(rule.id)}
                onOpenHistory={() => openHistory(rule.id)}
                onRequestDelete={() => setIsConfirmingDelete(true)}
            />

            <ConfirmDialog
                isOpen={isConfirmingDelete}
                onClose={() => setIsConfirmingDelete(false)}
                onConfirm={() => {
                    remove.mutate(rule.id)
                    setIsConfirmingDelete(false)
                }}
                title={`Delete "${rule.name}"?`}
                message="This cannot be undone."
                confirmLabel="Delete"
                isDestructive
                isSubmitting={remove.isPending}
            />
        </View>
    )
}

function RuleRowMain({
    rule,
    summary,
    missingPkg,
    showScopeBadge,
    lastRun,
}: {
    rule: Rules
    summary: string
    missingPkg: string | null
    showScopeBadge: boolean
    lastRun?: { fired_at: string; matched: boolean }
}) {
    return (
        <View className="flex-1 gap-0.5 min-w-0">
            <View className="flex-row items-center gap-2">
                <Text className="text-sm font-semibold text-foreground" numberOfLines={1}>
                    {rule.name}
                </Text>
                <ScopeBadge isVisible={showScopeBadge} />
                <NeedsPackageBadge pkg={missingPkg} />
            </View>
            <Text className="text-muted text-sm" numberOfLines={1}>
                {summary}
            </Text>
            <LastRunLine lastRun={lastRun} />
        </View>
    )
}

function ScopeBadge({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View className="px-1.5 py-0.5 rounded-full bg-info">
            <Text className="text-[10px] font-semibold text-info-foreground">Organization</Text>
        </View>
    )
}

function NeedsPackageBadge({ pkg }: { pkg: string | null }) {
    if (!pkg) return null
    return (
        <View className="px-1.5 py-0.5 rounded-full bg-warning-soft">
            <Text className="text-[10px] font-semibold text-warning-soft-foreground">
                needs {pkg}
            </Text>
        </View>
    )
}

function LastRunLine({ lastRun }: { lastRun?: { fired_at: string; matched: boolean } }) {
    const mutedColor = useThemeColor('muted-foreground')
    if (!lastRun) return null
    return (
        <View className="flex-row items-center gap-1">
            <Clock size={11} color={mutedColor} />
            <Text className="text-[11px] text-muted-foreground">
                {lastRun.matched ? 'Ran' : "Didn't match"} {formatRelativeTime(lastRun.fired_at)}
            </Text>
        </View>
    )
}

function RuleRowMenu({
    rule,
    canEdit,
    canRunNow,
    onEdit,
    onOpenHistory,
    onRequestDelete,
}: {
    rule: Rules
    canEdit: boolean
    canRunNow: boolean
    onEdit: () => void
    onOpenHistory: () => void
    onRequestDelete: () => void
}) {
    const { runNow } = useRuleMutations()
    const [isQueued, setIsQueued] = useState(false)

    // notify.emit requires a registered event name (no "rule run queued" event
    // exists in the typed registry), so feedback is transient local UI state
    // rather than a toast — the menu item label swaps to "Queued ✓" for 2s.
    const handleRunNow = () => {
        runNow.mutate(rule.id)
        setIsQueued(true)
        setTimeout(() => setIsQueued(false), 2000)
    }

    return (
        <DotsMenu>
            <MenuActionItem label="Edit" icon={Pencil} onPress={onEdit} disabled={!canEdit} />
            <MenuActionItem label="Run history" icon={History} onPress={onOpenHistory} />
            <RunNowMenuItem
                isVisible={canRunNow && canEdit}
                isQueued={isQueued}
                onPress={handleRunNow}
            />
            <MenuSeparator />
            <MenuActionItem
                label="Delete"
                icon={Trash2}
                onPress={onRequestDelete}
                disabled={!canEdit}
            />
        </DotsMenu>
    )
}

function RunNowMenuItem({
    isVisible,
    isQueued,
    onPress,
}: {
    isVisible: boolean
    isQueued: boolean
    onPress: () => void
}) {
    if (!isVisible) return null
    return (
        <MenuActionItem label={isQueued ? 'Queued ✓' : 'Run now'} icon={Play} onPress={onPress} />
    )
}
