import type { CatalogResponse, DryRunResponse } from '@tinycld/core/lib/automation/api'
import type { RuleDraft } from '@tinycld/core/lib/automation/draft'
import { conditionsAstSchema } from '@tinycld/core/lib/automation/schemas'
import { useRuleMutations } from '@tinycld/core/lib/automation/use-rule-mutations'
import { errorToString } from '@tinycld/core/lib/errors'
import { Button, ButtonSpinner, ButtonText } from '@tinycld/core/ui/button'
import { Text, View } from 'react-native'

export interface DryRunPanelProps {
    draft: RuleDraft
    catalog: CatalogResponse
    isVisible: boolean
}

const MAX_MATCH_ROWS = 10

function matchSummaryText(match: DryRunResponse['matches'][number], fieldKeys: string[]): string {
    const parts = fieldKeys
        .map(key => match.summary[key])
        .filter(v => v !== undefined && v !== null)
        .map(String)
    return parts.length > 0 ? parts.join(' · ') : match.id
}

function MatchRow({
    match,
    fieldKeys,
}: {
    match: DryRunResponse['matches'][number]
    fieldKeys: string[]
}) {
    return <Text className="text-sm text-foreground">{matchSummaryText(match, fieldKeys)}</Text>
}

function ResultSummary({ result, fieldKeys }: { result: DryRunResponse; fieldKeys: string[] }) {
    return (
        <View className="gap-1.5">
            <Text className="text-sm text-foreground">
                Matched {result.matches.length} of the last {result.total}
            </Text>
            {result.matches.slice(0, MAX_MATCH_ROWS).map(match => (
                <MatchRow key={match.id} match={match} fieldKeys={fieldKeys} />
            ))}
        </View>
    )
}

// The 400 "cannot be scoped" case (core/server/automation/endpoints.go's
// dryRun: a trigger whose owner can't be resolved to the caller, e.g. no
// direct user/owner/author column) is expected, common for non-admins, and
// not a bug — it renders as an informational row, never an error toast.
// Distinguishing it from a genuine failure (bad trigger ref, server error)
// is deferred to Phase 2 per the task brief; for now every dry-run error
// renders this same muted message rather than surfacing two code paths.
function ErrorNotice({ error }: { error: unknown }) {
    return (
        <View className="gap-1">
            <Text className="text-sm text-muted-foreground">{errorToString(error)}</Text>
            <Text className="text-sm text-muted-foreground">Org admins can test this trigger.</Text>
        </View>
    )
}

export function DryRunPanel({ draft, catalog, isVisible }: DryRunPanelProps) {
    // dryRun's onError is an explicit no-op: the mutation's default onError
    // (see use-mutation.ts's reportUnhandledMutationError) would toast + report
    // to Sentry on every "cannot be scoped" 400, which is an expected outcome
    // for a non-admin testing someone else's trigger, not a bug — the panel
    // reads `error` directly below and renders it as an informational row
    // instead. See core/server/automation/endpoints.go's dryRun for the
    // server-side condition this suppresses.
    const { dryRun } = useRuleMutations()

    if (!isVisible) return null
    const trigger = catalog.triggers.find(t => t.ref === draft.trigger)
    if (!trigger || trigger.synthetic) return null

    const fieldKeys = (trigger.fields ?? []).slice(0, 2).map(f => f.key)

    const handleTest = () => {
        dryRun.mutate(
            {
                trigger: draft.trigger,
                // conditionsAstSchema.parse strips the builder-local `uid` key
                // (see condition-helpers.ts) before the AST is sent — the
                // server's ConditionsAST has no such field.
                conditions: conditionsAstSchema.parse(draft.conditions),
            },
            { onError: () => {} }
        )
    }

    return (
        <View className="rounded-xl border p-4 bg-surface-secondary border-border gap-3">
            <Button variant="outline" size="sm" onPress={handleTest} disabled={dryRun.isPending}>
                {dryRun.isPending ? <ButtonSpinner /> : null}
                <ButtonText>Test against recent items</ButtonText>
            </Button>

            {dryRun.error ? <ErrorNotice error={dryRun.error} /> : null}
            {dryRun.data ? <ResultSummary result={dryRun.data} fieldKeys={fieldKeys} /> : null}
        </View>
    )
}
