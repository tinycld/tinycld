// The rules list: header ("+ New rule"), rows (sortable when canEdit), empty
// state, and the panel-level RuleBuilder + RunHistory instances the store
// wires open/closed. Requires a GestureHandlerRootView ancestor (SortableList
// dependency) — the mount screens (Settings → Rules, mail's Rules screen)
// provide it.
import { and, eq } from '@tanstack/db'
import { EmptyState } from '@tinycld/core/components/EmptyState'
import { RuleBuilder } from '@tinycld/core/components/rules/RuleBuilder'
import { RuleRow } from '@tinycld/core/components/rules/RuleRow'
import { RunHistory } from '@tinycld/core/components/rules/RunHistory'
import { SortableList } from '@tinycld/core/components/SortableList'
import type { CatalogResponse } from '@tinycld/core/lib/automation/api'
import { parseRef } from '@tinycld/core/lib/automation/helpers'
import { useAutomationCatalog } from '@tinycld/core/lib/automation/use-automation-catalog'
import { useRuleMutations } from '@tinycld/core/lib/automation/use-rule-mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useRulesUiStore } from '@tinycld/core/lib/stores/rules-ui-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import type { Rules } from '@tinycld/core/types/pbSchema'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Plus } from 'lucide-react-native'
import { useMemo } from 'react'
import { ActivityIndicator, View } from 'react-native'

export interface RulesPanelProps {
    scope: 'personal' | 'org'
    pkgFilter?: string
    canEdit: boolean
}

function useScopedRules(scope: 'personal' | 'org') {
    const [rulesCollection] = useStore('rules')
    return useOrgLiveQuery(
        (query, { userId }) => {
            if (scope === 'personal') {
                return query
                    .from({ r: rulesCollection })
                    .where(({ r }) => and(eq(r.scope, 'personal'), eq(r.owner, userId)))
            }
            return query.from({ r: rulesCollection }).where(({ r }) => eq(r.scope, 'org'))
        },
        [scope]
    )
}

// A single last-run-per-rule query for the whole visible set — never one
// query per row. Joins rule_runs to rules (mail/hooks/useMailboxes.ts's join
// idiom) and reduces to the newest row per rule client-side, since TanStack
// DB joins can't express "latest row per group" directly.
function useLastRunByRule(rules: Rules[]) {
    const [rulesCollection, runsCollection] = useStore('rules', 'rule_runs')
    const ruleIds = useMemo(() => new Set(rules.map(r => r.id)), [rules])

    const { data: rows } = useOrgLiveQuery(
        query =>
            query
                .from({ run: runsCollection })
                .innerJoin({ rule: rulesCollection }, ({ run, rule }) => eq(run.rule, rule.id)),
        []
    )

    return useMemo(() => {
        const latest = new Map<string, { fired_at: string; matched: boolean }>()
        for (const row of rows ?? []) {
            const ruleId = row.rule.id
            if (!ruleIds.has(ruleId)) continue
            const existing = latest.get(ruleId)
            if (!existing || row.run.fired_at > existing.fired_at) {
                latest.set(ruleId, { fired_at: row.run.fired_at, matched: row.run.matched })
            }
        }
        return latest
    }, [rows, ruleIds])
}

function filterByPkg(rules: Rules[], pkgFilter: string | undefined): Rules[] {
    if (!pkgFilter) return rules
    return rules.filter(r => {
        try {
            return parseRef(r.trigger).pkg === pkgFilter
        } catch {
            return false
        }
    })
}

function sortByOrder(rules: Rules[]): Rules[] {
    return [...rules].sort((a, b) => a.order - b.order)
}

export function RulesPanel({ scope, pkgFilter, canEdit }: RulesPanelProps) {
    const { data: rawRules, isReady } = useScopedRules(scope)
    const { catalog, isReady: catalogReady } = useAutomationCatalog()
    const { reorder } = useRuleMutations()
    const openCreate = useRulesUiStore(s => s.openCreate)
    const builder = useRulesUiStore(s => s.builder)
    const closeBuilder = useRulesUiStore(s => s.closeBuilder)
    const historyRuleId = useRulesUiStore(s => s.historyRuleId)
    const closeHistory = useRulesUiStore(s => s.closeHistory)

    const rules = useMemo(
        () => sortByOrder(filterByPkg(rawRules ?? [], pkgFilter)),
        [rawRules, pkgFilter]
    )
    const lastRunByRule = useLastRunByRule(rules)

    if (!isReady || !catalogReady) return <RulesPanelLoading />
    if (!catalog) return <RulesPanelCatalogError />

    return (
        <View className="gap-3">
            <RulesPanelHeader canEdit={canEdit} onNewRule={() => openCreate(scope, pkgFilter)} />

            <RulesPanelBody
                rules={rules}
                catalog={catalog}
                canEdit={canEdit}
                scope={scope}
                lastRunByRule={lastRunByRule}
                onReorder={ids => reorder.mutate(ids)}
                onCreate={() => openCreate(scope, pkgFilter)}
            />

            <RuleBuilder
                isOpen={builder.mode !== 'closed'}
                onClose={closeBuilder}
                scope={builder.mode === 'create' ? builder.scope : scope}
                ruleId={builder.mode === 'edit' ? builder.ruleId : undefined}
                presetPkg={builder.mode === 'create' ? builder.presetPkg : undefined}
            />
            <RunHistory ruleId={historyRuleId} onClose={closeHistory} />
        </View>
    )
}

function RulesPanelHeader({ canEdit, onNewRule }: { canEdit: boolean; onNewRule: () => void }) {
    return (
        <View className="flex-row items-center justify-end">
            <NewRuleButton isVisible={canEdit} onPress={onNewRule} />
        </View>
    )
}

function NewRuleButton({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    if (!isVisible) return null
    return (
        <Button onPress={onPress} size="sm">
            <Plus size={16} />
            <ButtonText>New rule</ButtonText>
        </Button>
    )
}

interface RulesPanelBodyProps {
    rules: Rules[]
    catalog: CatalogResponse
    canEdit: boolean
    scope: 'personal' | 'org'
    lastRunByRule: Map<string, { fired_at: string; matched: boolean }>
    onReorder: (orderedIds: string[]) => void
    onCreate: () => void
}

function RulesPanelBody({
    rules,
    catalog,
    canEdit,
    scope,
    lastRunByRule,
    onReorder,
    onCreate,
}: RulesPanelBodyProps) {
    if (rules.length === 0) {
        return (
            <EmptyState
                message="No rules yet — automate repetitive steps with a rule."
                action={canEdit ? { label: 'New rule', onPress: onCreate } : undefined}
            />
        )
    }

    if (canEdit) {
        return (
            <SortableList
                data={rules}
                keyExtractor={rule => rule.id}
                onReorder={items => onReorder(items.map(r => r.id))}
                renderItem={({ item }) => (
                    <RuleRow
                        rule={item}
                        catalog={catalog}
                        canEdit={canEdit}
                        lastRun={lastRunByRule.get(item.id)}
                        showScopeBadge={scope === 'org'}
                    />
                )}
            />
        )
    }

    return (
        <View>
            {rules.map(rule => (
                <RuleRow
                    key={rule.id}
                    rule={rule}
                    catalog={catalog}
                    canEdit={canEdit}
                    lastRun={lastRunByRule.get(rule.id)}
                    showScopeBadge={scope === 'org'}
                />
            ))}
        </View>
    )
}

function RulesPanelLoading() {
    const accent = useThemeColor('primary')
    return (
        <View className="items-center justify-center p-10">
            <ActivityIndicator size="large" color={accent} />
        </View>
    )
}

function RulesPanelCatalogError() {
    return <EmptyState message="Couldn't load the automation catalog. Try again in a moment." />
}
