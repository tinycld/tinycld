// Authoring types for a package's automation.ts (pure data, spec:
// docs/superpowers/specs/2026-08-11-workflow-rules-design.md). The S generic is
// the package's generated schema map ({ collection: { type, relations } }), so
// collection and field references are compile-checked in the package.

export type FieldType = 'text' | 'number' | 'boolean' | 'date' | 'select' | 'relation'

export type ConditionOp =
    | 'contains'
    | 'not_contains'
    | 'equals'
    | 'starts_with'
    | 'eq'
    | 'neq'
    | 'gt'
    | 'lt'
    | 'is_true'
    | 'is_false'
    | 'before'
    | 'after'
    | 'within_last_days'
    | 'is'
    | 'is_not'
    | 'is_empty'

type AnySchema = Record<string, { type: Record<string, unknown> }>
type CollectionsOf<S> = keyof S & string
type FieldsOf<S, C extends keyof S> = S[C] extends { type: infer T } ? keyof T & string : string

/** A trigger field entry: bare column key, or key + display-label override. */
export type FieldRef<F extends string = string> = F | { key: F; label: string }

export interface RecordTriggerDefBase<C extends string, F extends string> {
    id: string
    label: string
    collection: C
    on: 'create' | 'update' | 'delete'
    /** update triggers only: fire only when one of these columns changed */
    watch?: F[]
    /** omitted = expose every schema column (see spec: contract rules) */
    fields?: FieldRef<F>[]
    /** override the auto-detected user/owner/author owner column */
    ownerField?: F
}

export type RecordTriggerDef<S = AnySchema> = {
    [C in CollectionsOf<S>]: RecordTriggerDefBase<C, FieldsOf<S, C>>
}[CollectionsOf<S>]

/**
 * Core-only synthetic triggers with no backing record (core:schedule,
 * core:manual). Declared by core's own catalog; the generator rejects them in
 * feature packages.
 */
export interface SyntheticTriggerDef {
    id: string
    label: string
    synthetic: 'schedule' | 'manual'
}

export type TriggerDef<S = AnySchema> = RecordTriggerDef<S> | SyntheticTriggerDef

/**
 * A value written by a record-op `set` entry:
 * - `{ param }`: the rule author supplies it (static value or template)
 * - `{ context }`: engine-provided — the trigger record's id, its collection
 *   name, or the executing rule's owner id
 * - literal: fixed at declaration time
 */
export type SetValue =
    | { param: string }
    | { context: 'record-id' | 'collection' | 'owner' }
    | string
    | number
    | boolean

export type RecordOp<F extends string = string> =
    | { type: 'update'; target: 'trigger-record'; set: Partial<Record<F, SetValue>> }
    | { type: 'delete'; target: 'trigger-record' }
    | { type: 'create'; set: Partial<Record<F, SetValue>> }

/** Column-referencing param (type/relation resolved from the column in Phase 2) */
export interface ColumnParamDef<F extends string = string> {
    key: string
    field: F
    label?: string
}

/** Novel param (not a DB column) — declares its own type */
export interface TypedParamDef {
    key: string
    type: FieldType
    label?: string
    options?: string[]
    /**
     * Required when `type` is 'relation' (validateDefinitions enforces both
     * directions): the target collection NAME the record picker lists. A
     * column-referencing param inherits its target from the column instead;
     * this field exists so native actions — which reference no collection —
     * can offer a picker at all.
     */
    relationTarget?: string
}

export type ParamDef<F extends string = string> = ColumnParamDef<F> | TypedParamDef

export type RecordOpActionDef<S = AnySchema> = {
    [C in CollectionsOf<S>]: {
        id: string
        label: string
        kind: 'record-op'
        collection: C
        op: RecordOp<FieldsOf<S, C>>
        params?: ParamDef<FieldsOf<S, C>>[]
    }
}[CollectionsOf<S>]

export interface NativeActionDef {
    id: string
    label: string
    kind: 'native'
    params?: TypedParamDef[]
}

export type ActionDef<S = AnySchema> = RecordOpActionDef<S> | NativeActionDef

export interface AutomationDefinitions<S = AnySchema> {
    triggers?: TriggerDef<S>[]
    actions?: ActionDef<S>[]
}
