import type { SpecNode } from '../spec'
import type { FieldType, ParamType } from '../types'
import { type Catalog, describePath } from './columns'
import {
    type FormulaParam,
    type FormulaRecord,
    lookupValueNode,
    partOf,
    type ReportFormulaContext,
} from './formula-parts'

/** A formula's result type; blank for a result that is always empty (the engine's untyped NULL). */
export type FormulaType = FieldType | ''

/** What a formula's type is read from. */
export interface FormulaTypes {
    catalog: Catalog
    source: string | null
    /** The report's source, which an {outer} path reads; absent reads as none. */
    rowSource?: string | null
    col: (key: string) => FieldType | undefined
    records: readonly FormulaRecord[]
    params: readonly Pick<FormulaParam, 'id' | 'type' | 'multiple'>[]
}

export function typesOf(ctx: ReportFormulaContext): FormulaTypes {
    return {
        catalog: ctx.catalog,
        source: ctx.source,
        rowSource: ctx.rowSource,
        col: key => ctx.values.find(v => v.key === key)?.type,
        records: ctx.records,
        params: ctx.params,
    }
}

// params.go paramExpr: a date range is read only by between, as dates.
const PARAM_TYPES: Record<ParamType, FieldType> = {
    date: 'date',
    asOf: 'date',
    dateRange: 'date',
    number: 'number',
    text: 'text',
    choice: 'text',
    amount: 'money',
}

const BOOL_PARTS: ReadonlySet<string> = new Set([
    'eq',
    'ne',
    'lt',
    'lte',
    'gt',
    'gte',
    'between',
    'inPeriod',
    'in',
    'and',
    'or',
    'not',
    'isEmpty',
    'contains',
    'startsWith',
    'exists',
    'notExists',
])

/** The formula's result type, as the engine types it (formula.go operatorExpr). */
export function formulaType(node: SpecNode, types: FormulaTypes): FormulaType {
    const t = (n: SpecNode | undefined): FormulaType => (n ? formulaType(n, types) : '')
    const args = node.args ?? []
    // sumType: an amount when any operand is one, else a percent when any is one.
    const sumType = (ns: SpecNode[]): FieldType => {
        const all = ns.map(t)
        if (all.includes('money')) return 'money'
        return all.includes('percent') ? 'percent' : 'number'
    }
    // commonType and withResults: blank results are skipped; an amount when any is
    // one; a select mixed with another kind is shown as text; else the first type.
    const commonType = (ns: SpecNode[]): FormulaType => {
        const all = ns.map(t).filter(x => x !== '')
        if (all.includes('money')) return 'money'
        if (all.includes('select') && all.some(x => x !== 'select')) return 'text'
        return all[0] ?? ''
    }
    const part = partOf(node)
    if (BOOL_PARTS.has(part)) return 'bool'
    switch (part) {
        case 'number':
            return 'number'
        case 'empty':
            return ''
        case 'text':
            return 'text'
        case 'list':
            return Array.isArray(node.value) && typeof node.value[0] === 'number'
                ? 'number'
                : 'text'
        case 'yesno':
            return 'bool'
        case 'field':
            return types.col(node.col ?? node.subtotal ?? '') ?? ''
        case 'path': {
            const info = types.source
                ? describePath(types.catalog, types.source, node.path ?? [])
                : null
            return info?.field.type ?? ''
        }
        case 'record': {
            const record = types.records.find(r => r.group === node.record)
            const info = record ? describePath(types.catalog, record.source, node.path ?? []) : null
            return info?.field.type ?? ''
        }
        case 'param': {
            const param = types.params.find(p => p.id === node.param)
            return param ? PARAM_TYPES[param.type] : ''
        }
        case 'outer': {
            const info = types.rowSource
                ? describePath(types.catalog, types.rowSource, node.outer ?? [])
                : null
            return info?.field.type ?? ''
        }
        case 'bound':
            return 'date'
        // lookup.go lookupIn: a count is a number; a sum keeps its value's type.
        case 'lookup':
            if (node.pick === 'count') return 'number'
            return formulaType(lookupValueNode(node), { ...types, source: node.source ?? null })
        case 'add':
        case 'sub':
        case 'min':
        case 'max':
        case 'runningSum':
            return sumType(args)
        case 'mul':
            return args.filter(a => t(a) === 'money').length === 1 ? 'money' : 'number'
        case 'div':
            return t(args[0]) === 'money' && t(args[1]) !== 'money' ? 'money' : 'number'
        // aggregateType: an average stays an amount only in whole cents.
        case 'avg':
            return t(args[0]) === 'money' && node.scale === 0 ? 'money' : 'number'
        case 'round': {
            const inner = t(args[0])
            return inner === 'bool' || inner === '' ? 'number' : inner
        }
        case 'sum':
        case 'smallest':
        case 'largest': {
            const inner = t(args[0])
            return inner === 'bool' ? 'number' : inner
        }
        case 'count':
        case 'countDistinct':
        case 'year':
        case 'quarter':
        case 'month':
        case 'fiscalYear':
        case 'fiscalPeriod':
        case 'diffDays':
            return 'number'
        case 'percentOfParent':
        case 'percentOfTotal':
            return 'percent'
        case 'addDays':
        case 'addMonths':
            return 'date'
        case 'concat':
        case 'upper':
        case 'lower':
            return 'text'
        case 'if':
            return commonType(args.slice(1))
        case 'case': {
            const hasElse = args.length % 2 === 1
            const results = args.filter((_, i) => i % 2 === 1 || (hasElse && i === args.length - 1))
            return commonType(results)
        }
        case 'coalesce':
            return commonType(args)
    }
    return ''
}
