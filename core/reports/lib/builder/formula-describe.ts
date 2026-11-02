import type { SpecHop, SpecNode } from '../spec'
import { type Catalog, describePath, pathLabel } from './columns'
import { onlyKeys } from './formula'
import { contextAt } from './formula-ops'
import {
    lookupValueNode,
    partOf,
    REPORT_TREE,
    type ReportFormulaContext,
    type ReportPart,
} from './formula-parts'

// A report formula read back as one line of text, by label, for the editor's
// summary line and a read-only formula.

const SIGNS: Partial<Record<string, string>> = {
    add: ' + ',
    sub: ' − ',
    mul: ' × ',
    div: ' ÷ ',
    eq: ' = ',
    ne: ' ≠ ',
    lt: ' < ',
    lte: ' ≤ ',
    gt: ' > ',
    gte: ' ≥ ',
}
const WORDS: Partial<Record<string, string>> = {
    sum: 'sum of',
    count: 'count of',
    countDistinct: 'distinct count of',
    avg: 'average of',
    smallest: 'smallest',
    largest: 'largest',
    runningSum: 'running sum of',
    upper: 'upper case of',
    lower: 'lower case of',
    year: 'year of',
    quarter: 'quarter of',
    month: 'month of',
    fiscalYear: 'fiscal year of',
    fiscalPeriod: 'fiscal period of',
    not: 'not',
}

/** The labels of an exists link's hops: "Sales", "Team › Sales". */
export function linkLabel(
    catalog: Catalog,
    sourceKey: string | null,
    links: readonly SpecHop[]
): string {
    const labels: string[] = []
    let source = sourceKey ? catalog.get(sourceKey) : undefined
    for (const hop of links) {
        const link = source?.links.find(l => l.key === hop.link)
        if (!link) return `${hop.link} (missing)`
        labels.push(link.label)
        source = catalog.get(link.target)
    }
    return labels.join(' › ')
}

const matchesAll = (where: SpecNode) => where.value === true && onlyKeys(where, ['value'])

function describeLeaf(here: ReportFormulaContext, n: SpecNode, part: ReportPart): string | null {
    switch (part) {
        case 'number':
            return String(n.value)
        case 'text':
            return JSON.stringify(n.value)
        case 'yesno':
            return n.value ? 'yes' : 'no'
        case 'empty':
            return 'empty'
        case 'list': {
            const items = Array.isArray(n.value) ? n.value : []
            return `[${items.map(v => JSON.stringify(v)).join(', ')}]`
        }
        case 'field': {
            const key = n.col ?? n.subtotal ?? ''
            return here.values.find(v => v.key === key)?.label ?? `${key} (missing)`
        }
        case 'path': {
            const info = here.source ? describePath(here.catalog, here.source, n.path ?? []) : null
            return info ? pathLabel(info) : 'a field (missing)'
        }
        case 'record': {
            const record = here.records.find(r => r.group === n.record)
            const info = record ? describePath(here.catalog, record.source, n.path ?? []) : null
            return record && info ? `${record.label} › ${pathLabel(info)}` : `${n.record} (missing)`
        }
        case 'param':
            return `{${here.params.find(p => p.id === n.param)?.label ?? n.param}}`
    }
    return null
}

/** The formula as one line of text, by label: "Amount − Cost", "if Amount > Cost then Amount else Cost". */
export function describeNode(ctx: ReportFormulaContext, node: SpecNode): string {
    const say = (n: SpecNode, here: ReportFormulaContext, nested: boolean): string => {
        const part = partOf(n)
        if (part === 'unknown') return 'a part the editor cannot show'
        const leaf = describeLeaf(here, n, part)
        if (leaf !== null) return leaf
        const args = n.args ?? []
        const sub = (a: SpecNode | undefined) => (a ? say(a, here, true) : '?')
        const flat = (a: SpecNode | undefined) => (a ? say(a, here, false) : '?')
        const list = () => args.map(a => say(a, here, false)).join(', ')
        const wrap = (text: string) => (nested ? `(${text})` : text)
        const whereText = (where: SpecNode | undefined, inside: ReportFormulaContext) =>
            where === undefined || matchesAll(where) ? '' : ` where ${say(where, inside, false)}`
        switch (part) {
            case 'lookup': {
                const inside = contextAt(here, n, [1])
                const [condition] = REPORT_TREE.childrenOf(n)
                const source = here.catalog.get(n.source ?? '')?.label ?? `${n.source} (missing)`
                const where = whereText(condition, inside)
                const pick = n.pick === 'max' ? 'largest' : 'smallest'
                return `look up ${say(lookupValueNode(n), inside, false)} in ${source}${where} (${pick})`
            }
            case 'exists':
            case 'notExists': {
                const inside = contextAt(here, n, [0])
                const has = part === 'exists' ? 'has related' : 'has no related'
                const links = linkLabel(here.catalog, here.source, n.link ?? [])
                return wrap(`${has} ${links}${whereText(n.where, inside)}`)
            }
            case 'if':
                return wrap(`if ${flat(args[0])} then ${flat(args[1])} else ${flat(args[2])}`)
            case 'round':
                return `round(${flat(args[0])}, ${n.scale})`
            case 'min':
            case 'max':
                return `${part === 'min' ? 'smallest' : 'largest'} of (${list()})`
            case 'and':
            case 'or':
                return wrap(args.map(a => sub(a)).join(` ${part} `))
            case 'isEmpty':
                return wrap(`${sub(args[0])} is empty`)
            case 'between':
                return wrap(`${sub(args[0])} between ${sub(args[1])} and ${sub(args[2])}`)
            case 'inPeriod':
                return wrap(`${sub(args[0])} is in ${sub(args[1])}`)
            case 'in':
                return wrap(`${sub(args[0])} is any of ${sub(args[1])}`)
            case 'case':
                return `first match of (${list()})`
            case 'coalesce':
                return `first not empty of (${list()})`
            case 'concat':
                return args.map(a => sub(a)).join(' & ')
            case 'contains':
            case 'startsWith':
                return wrap(
                    `${sub(args[0])} ${part === 'contains' ? 'contains' : 'starts with'} ${sub(args[1])}`
                )
            case 'addDays':
            case 'addMonths':
                return wrap(
                    `${sub(args[0])} + ${sub(args[1])} ${part === 'addDays' ? 'days' : 'months'}`
                )
            case 'diffDays':
                return `days from ${sub(args[0])} to ${sub(args[1])}`
            case 'percentOfParent':
            case 'percentOfTotal':
                return `${sub(args[0])} as % of ${part === 'percentOfParent' ? 'group' : 'total'}`
        }
        const sign = SIGNS[part]
        if (sign) return wrap(args.map(a => sub(a)).join(sign))
        const word = WORDS[part]
        if (word) return `${word} ${sub(args[0])}`
        return 'a part the editor cannot show'
    }
    return say(node, ctx, false)
}
