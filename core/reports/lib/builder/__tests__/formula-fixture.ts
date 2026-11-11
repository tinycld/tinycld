import type { SpecNode } from '../../spec'
import type { ReportFormulaContext } from '../formula-parts'
import { CATALOG, SALES_KEY, TEAMS_KEY } from './catalog-fixture'

// Formula contexts and formulas the report formula tests share.

export const CTX: ReportFormulaContext = {
    catalog: CATALOG,
    leaf: 'col',
    values: [
        { key: 'amount', label: 'Amount', type: 'money' },
        { key: 'cost', label: 'Cost', type: 'money' },
        { key: 'qty', label: 'Quantity', type: 'number' },
    ],
    source: SALES_KEY,
    rowSource: SALES_KEY,
    records: [{ group: 'byTeam', label: 'Team', source: TEAMS_KEY }],
    params: [{ id: 'asOf', label: 'As of', type: 'asOf' }],
    totals: true,
    windows: true,
    lookups: true,
    sections: [],
}
// A computed row reads params (Ruling 7) and looks values up (F16). It has no
// report row, so "This row's field" is not offered there.
export const LAYOUT: ReportFormulaContext = {
    ...CTX,
    leaf: 'subtotal',
    source: null,
    rowSource: null,
    records: [],
    totals: false,
    windows: false,
    lookups: true,
}
export const WITH_PERIOD: ReportFormulaContext = {
    ...CTX,
    params: [
        { id: 'period', label: 'Period', type: 'dateRange' },
        { id: 'asOf', label: 'As of', type: 'asOf' },
        { id: 'statuses', label: 'Statuses', type: 'choice', multiple: true },
    ],
}

export const LARGER: SpecNode = {
    op: 'if',
    args: [
        { op: 'gt', args: [{ col: 'amount' }, { col: 'cost' }] },
        { col: 'amount' },
        { col: 'cost' },
    ],
}
export const DEBIT: SpecNode = {
    op: 'if',
    args: [
        { op: 'eq', args: [{ op: 'count', args: [{ path: [{ field: 'id' }] }] }, { value: 0 }] },
        { value: null },
        {
            op: 'if',
            args: [
                {
                    op: 'gte',
                    args: [{ op: 'sum', args: [{ path: [{ field: 'amount' }] }] }, { value: 0 }],
                },
                { op: 'sum', args: [{ path: [{ field: 'amount' }] }] },
                { value: null },
            ],
        },
    ],
}
export const LOOKUP: SpecNode = {
    op: 'lookup',
    source: TEAMS_KEY,
    where: { op: 'eq', args: [{ path: [{ field: 'name' }] }, { value: 'North' }] },
    value: { path: [{ field: 'id' }] },
    pick: 'min',
}
export const IN_PERIOD: SpecNode = {
    op: 'between',
    args: [{ path: [{ field: 'day' }] }, { param: 'period' }],
}
/** A team that has a sale over 0: an exists inside a lookup's where (lookup.go lookupCtx.exists). */
export const TEAM_WITH_SALES: SpecNode = {
    op: 'lookup',
    source: TEAMS_KEY,
    where: {
        op: 'exists',
        link: [{ link: 'sales' }],
        where: { op: 'gt', args: [{ path: [{ field: 'amount' }] }, { value: 0 }] },
    },
    value: { path: [{ field: 'name' }] },
}

/** A column of a report with two layout sections, and a text column beside the numbers. */
export const WITH_SECTIONS: ReportFormulaContext = {
    ...CTX,
    values: [...CTX.values, { key: 'team', label: 'Team', type: 'text' }],
    sections: [
        { id: 'open', title: 'Open sales' },
        { id: 'paid', title: 'Paid sales' },
    ],
}
export const SHARE: SpecNode = {
    op: 'percentOfSection',
    args: [{ col: 'amount' }],
    subtotal: 'open',
    scale: 1,
}
