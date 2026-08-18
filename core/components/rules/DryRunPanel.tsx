import type { CatalogResponse, DryRunMatch, DryRunResult } from '@tinycld/core/lib/automation/api'
import type { RuleDraft } from '@tinycld/core/lib/automation/draft'
import { conditionsAstSchema } from '@tinycld/core/lib/automation/schemas'
import { useRuleMutations } from '@tinycld/core/lib/automation/use-rule-mutations'
import { errorToString } from '@tinycld/core/lib/errors'
import { Button, ButtonSpinner, ButtonText } from '@tinycld/core/ui/button'
import { useMemo } from 'react'
import { Text, View } from 'react-native'

export interface DryRunPanelProps {
    draft: RuleDraft
    catalog: CatalogResponse
    isVisible: boolean
}

const MAX_MATCH_ROWS = 10

function matchSummaryText(match: DryRunMatch, fieldKeys: string[]): string {
    const parts = fieldKeys
        .map(key => match.summary[key])
        .filter(v => v !== undefined && v !== null)
        .map(String)
    return parts.length > 0 ? parts.join(' · ') : match.id
}

function MatchRow({ match, fieldKeys }: { match: DryRunMatch; fieldKeys: string[] }) {
    return <Text className="text-sm text-foreground">{matchSummaryText(match, fieldKeys)}</Text>
}

function ResultSummary({ result, fieldKeys }: { result: DryRunResult; fieldKeys: string[] }) {
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

// The engine's dryRun always answers with HTTP 400 (core/server/automation/
// endpoints.go wraps every error — unknown trigger, unscopable owner, a
// FindRecordsByFilter failure — in the same BadRequestError), so there's no
// status code to branch on client-side. The unscopable case has one fixed
// message (engine.dryRun's "cannot be scoped to you" error) — match on that
// text to decide whether the "ask an admin" follow-up applies; any other
// message (bad trigger ref, genuine server error) renders alone. PocketBase's
// router runs every ApiError message through inflector.Sentenize (uppercases
// the first letter, appends a trailing period) before it hits the wire, so
// this matches case-insensitively on the stable middle of the sentence
// rather than the literal Go source string.
const UNSCOPABLE_MESSAGE_FRAGMENT = 'cannot be scoped to you'

function ErrorNotice({ error }: { error: unknown }) {
    const message = errorToString(error)
    const isUnscopable = message.toLowerCase().includes(UNSCOPABLE_MESSAGE_FRAGMENT)
    return (
        <View className="gap-1">
            <Text className="text-sm text-muted-foreground">{message}</Text>
            {isUnscopable ? (
                <Text className="text-sm text-muted-foreground">
                    Org admins can test this trigger.
                </Text>
            ) : null}
        </View>
    )
}

export function DryRunPanel({ draft, catalog, isVisible }: DryRunPanelProps) {
    if (!isVisible) return null
    const trigger = catalog.triggers.find(t => t.ref === draft.trigger)
    if (!trigger || trigger.synthetic) return null

    // Keying on trigger + a digest of the conditions remounts the content
    // below whenever either changes — the simplest way to drop a stale dry-run
    // result/error without an effect: useMutation's data/error live in that
    // mount's local state, so a fresh key means a fresh (empty) mutation
    // state, same as if the user had never pressed Test.
    //
    // The digest deliberately excludes the builder-local `uid`s that
    // JSON.stringify(draft.conditions) would carry: those are fresh React keys
    // regenerated as groups and rows are added, so including them remounted the
    // panel — discarding an in-flight request and its result — on edits that
    // did not change what would be matched at all.
    const conditionsKey = conditionsDigest(draft.conditions)

    return (
        <DryRunPanelContent
            key={`${draft.trigger}:${conditionsKey}`}
            draft={draft}
            trigger={trigger}
        />
    )
}

// conditionsDigest reduces an AST to what a dry run actually depends on: the
// match modes and each condition's field/op/value, in order. Two ASTs that
// would query identically produce the same digest.
function conditionsDigest(conditions: RuleDraft['conditions']): string {
    return JSON.stringify([
        conditions.match,
        conditions.groups.map(group => [
            group.match,
            group.conditions.map(c => [c.field, c.op, c.value ?? null]),
        ]),
    ])
}

function DryRunPanelContent({
    draft,
    trigger,
}: {
    draft: RuleDraft
    trigger: CatalogResponse['triggers'][number]
}) {
    // dryRun's onError is an explicit no-op: the mutation's default onError
    // (see use-mutation.ts's reportUnhandledMutationError) would toast + report
    // to Sentry on every "cannot be scoped" 400, which is an expected outcome
    // for a non-admin testing someone else's trigger, not a bug — the panel
    // reads `error` directly below and renders it as an informational row
    // instead. See core/server/automation/endpoints.go's dryRun for the
    // server-side condition this suppresses.
    const { dryRun } = useRuleMutations()
    const fieldKeys = useMemo(() => (trigger.fields ?? []).slice(0, 2).map(f => f.key), [trigger])

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
