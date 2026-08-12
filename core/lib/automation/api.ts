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

export interface DryRunResponse {
    total: number
    matches: Array<{
        id: string
        summary: Record<string, unknown>
    }>
}

export interface RunResponse {
    queued: boolean
}
