// Per-rule run history drawer: responsive like RuleBuilder (Modal on
// desktop/tablet, BottomDrawer on mobile — same split, same
// GestureHandlerRootView mount-region caveat: the mount screen provides it).
import { eq } from '@tanstack/db'
import { formatRelativeTime } from '@tinycld/core/components/NotificationDrawer'
import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import type { RuleRuns } from '@tinycld/core/types/pbSchema'
import { BottomDrawer } from '@tinycld/core/ui/bottom-drawer'
import { Modal, ModalBackdrop, ModalContent } from '@tinycld/core/ui/modal'
import { X } from 'lucide-react-native'
import { ActivityIndicator, Pressable, ScrollView, Text, View } from 'react-native'

export interface RunHistoryProps {
    ruleId: string | null
    onClose: () => void
}

interface RunActionResult {
    ref: string
    status: string
    message?: string
}

function useRuleRuns(ruleId: string | null) {
    const [runsCollection] = useStore('rule_runs')
    return useOrgLiveQuery(
        query => {
            if (!ruleId) return null
            return query.from({ r: runsCollection }).where(({ r }) => eq(r.rule, ruleId))
        },
        [ruleId]
    )
}

function sortRunsDesc(runs: RuleRuns[] | undefined): RuleRuns[] {
    if (!runs) return []
    // Equal fired_at must compare as 0 — the previous `? 1 : -1` claimed
    // b < a AND a < b for a tie (violates the comparator contract: sort
    // order for same-timestamp runs becomes engine-dependent/unstable).
    return [...runs].sort((a, b) => {
        if (a.fired_at === b.fired_at) return 0
        return b.fired_at > a.fired_at ? 1 : -1
    })
}

export function RunHistory({ ruleId, onClose }: RunHistoryProps) {
    const isMobile = useBreakpoint() === 'mobile'
    const isOpen = ruleId !== null

    if (isMobile) {
        return (
            <BottomDrawer isOpen={isOpen} onClose={onClose}>
                <View className="px-4 pb-4">
                    <RunHistoryContent ruleId={ruleId} onClose={onClose} />
                </View>
            </BottomDrawer>
        )
    }

    return (
        <Modal isOpen={isOpen} onClose={onClose}>
            <ModalBackdrop />
            <ModalContent>
                <RunHistoryContent ruleId={ruleId} onClose={onClose} />
            </ModalContent>
        </Modal>
    )
}

function RunHistoryContent({ ruleId, onClose }: RunHistoryProps) {
    const { data: rawRuns, isReady } = useRuleRuns(ruleId)
    const runs = sortRunsDesc(rawRuns)

    return (
        <View className="gap-3">
            <RunHistoryHeader onClose={onClose} />
            <ScrollView className="max-h-[70vh]">
                <RunHistoryBody isReady={isReady} runs={runs} />
            </ScrollView>
        </View>
    )
}

function RunHistoryHeader({ onClose }: { onClose: () => void }) {
    const mutedColor = useThemeColor('muted-foreground')
    return (
        <View className="flex-row items-center justify-between">
            <Text className="text-lg font-semibold text-foreground">Run history</Text>
            <Pressable onPress={onClose} accessibilityLabel="Close" className="p-1">
                <X size={18} color={mutedColor} />
            </Pressable>
        </View>
    )
}

function RunHistoryBody({ isReady, runs }: { isReady: boolean; runs: RuleRuns[] }) {
    if (!isReady) return <RunHistoryLoading />
    if (runs.length === 0) return <RunHistoryEmpty />
    return (
        <View className="gap-2 pb-2">
            {runs.map(run => (
                <RunRow key={run.id} run={run} />
            ))}
        </View>
    )
}

function RunHistoryLoading() {
    const accent = useThemeColor('primary')
    return (
        <View className="items-center justify-center p-8">
            <ActivityIndicator size="small" color={accent} />
        </View>
    )
}

function RunHistoryEmpty() {
    return (
        <View className="items-center justify-center p-8">
            <Text className="text-sm text-muted">No runs yet.</Text>
        </View>
    )
}

function RunRow({ run }: { run: RuleRuns }) {
    const results = parseResults(run.results)
    return (
        <View className="rounded-lg border border-border bg-surface-secondary p-3 gap-2">
            <View className="flex-row items-center justify-between">
                <MatchedPill matched={run.matched} />
                <Text className="text-xs text-muted-foreground">
                    {formatRelativeTime(run.fired_at)}
                </Text>
            </View>
            <Text className="text-xs text-muted-foreground">{run.duration_ms}ms</Text>
            <ActionResultsList results={results} />
            <RunError error={run.error} />
        </View>
    )
}

function MatchedPill({ matched }: { matched: boolean }) {
    if (matched) {
        return (
            <View className="px-2 py-0.5 rounded-full bg-success-soft">
                <Text className="text-[11px] font-semibold text-success-soft-foreground">
                    Matched
                </Text>
            </View>
        )
    }
    return (
        <View className="px-2 py-0.5 rounded-full bg-muted">
            <Text className="text-[11px] font-semibold text-muted-foreground">Didn't match</Text>
        </View>
    )
}

// A rule can run the same action ref more than once in a single run, so
// result.ref alone isn't a unique React key — disambiguate repeats by
// occurrence count (computed here, not from the map callback's index, so
// biome's noArrayIndexKey doesn't flag it) while keeping the key stable
// across re-renders of this render-only, never-reordered list.
function keyActionResults(results: RunActionResult[]): (RunActionResult & { key: string })[] {
    const seen = new Map<string, number>()
    return results.map(result => {
        const occurrence = seen.get(result.ref) ?? 0
        seen.set(result.ref, occurrence + 1)
        return { ...result, key: `${result.ref}#${occurrence}` }
    })
}

function ActionResultsList({ results }: { results: RunActionResult[] }) {
    if (results.length === 0) return null
    return (
        <View className="gap-1">
            {keyActionResults(results).map(result => (
                <ActionResultRow key={result.key} result={result} />
            ))}
        </View>
    )
}

function ActionResultRow({ result }: { result: RunActionResult }) {
    const isOk = result.status === 'ok'
    return (
        <View className="flex-row items-center gap-2">
            <Text className={`text-xs ${isOk ? 'text-success' : 'text-danger'}`}>
                {isOk ? '✓' : '✗'}
            </Text>
            <Text className="text-xs text-foreground flex-1">{result.ref}</Text>
            <RunActionMessage message={result.message} />
        </View>
    )
}

function RunActionMessage({ message }: { message?: string }) {
    if (!message) return null
    return <Text className="text-xs text-muted-foreground">{message}</Text>
}

function RunError({ error }: { error: string }) {
    if (!error) return null
    return <Text className="text-xs text-danger">{error}</Text>
}

function parseResults(value: unknown): RunActionResult[] {
    if (!Array.isArray(value)) return []
    const results: RunActionResult[] = []
    for (const item of value) {
        if (!item || typeof item !== 'object') continue
        const ref = (item as { ref?: unknown }).ref
        const status = (item as { status?: unknown }).status
        if (typeof ref !== 'string' || typeof status !== 'string') continue
        const message = (item as { message?: unknown }).message
        results.push({ ref, status, message: typeof message === 'string' ? message : undefined })
    }
    return results
}
