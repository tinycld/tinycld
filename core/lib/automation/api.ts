/** mirrors core/server/automation/catalog.go */

import type { z } from 'zod'

import type { conditionsAstSchema, ruleActionSchema } from './schemas'

export type ConditionsAst = z.infer<typeof conditionsAstSchema>
export type RuleActionItem = z.infer<typeof ruleActionSchema>

export interface CatalogField {
    key: string
    label: string
    type: 'text' | 'number' | 'boolean' | 'date' | 'select' | 'relation'
    options?: string[]
    relationTarget?: string
    displayField?: string
}

export interface CatalogParam {
    key: string
    label: string
    field: CatalogField
    template: boolean
}

export interface CatalogTrigger {
    ref: string
    pkg: string
    label: string
    synthetic?: string
    collection?: string
    fields?: CatalogField[]
}

export interface CatalogAction {
    ref: string
    pkg: string
    label: string
    kind: string
    collection?: string
    opType?: string
    opTarget?: string
    params?: CatalogParam[]
    available: boolean
}

export interface CatalogResponse {
    triggers: CatalogTrigger[]
    actions: CatalogAction[]
}

export interface DryRunRequest {
    trigger: string
    conditions: ConditionsAst
}

export interface DryRunMatch {
    id: string
    summary: Record<string, unknown>
}

// `matches` is nullable on the wire: Go marshals an empty slice as `null`
// unless it was explicitly initialized, and an older server on the other side
// of an upgrade still does. Typed honestly so the compiler forces the guard —
// consumers should read the normalized result from dryRun() rather than
// touching this shape directly.
export interface DryRunResponse {
    total: number
    matches: DryRunMatch[] | null
}

// DryRunResult is what callers actually get: matches is guaranteed present.
export interface DryRunResult {
    total: number
    matches: DryRunMatch[]
}

export interface RunResponse {
    queued: boolean
}
