import { bucketWords, isIsoDay, shortDay } from './dates'
import { formatMoney, groupDigits, MAX_STORED_AMOUNT, roundHalfAwayFromZero } from './money'
import {
    type CellValue,
    type FieldType,
    isBucketLabelType,
    type ResultColumn,
    type ResultRow,
    type RowKind,
} from './types'

// The Customize options of a run. They stay in the client (the server accepts and
// ignores options.display): formatting never changes a figure.
export interface DisplayOptions {
    divideBy1000: boolean
    withoutCents: boolean
    negative: 'minus' | 'parens'
    negativeRed: boolean
    /** Ids of columns the reader hid; the first column is never hidden. */
    hidden: string[]
}
export type NumberFormat = Omit<DisplayOptions, 'hidden'>

export const DEFAULT_DISPLAY: DisplayOptions = {
    divideBy1000: false,
    withoutCents: false,
    negative: 'minus',
    negativeRed: false,
    hidden: [],
}

export const REPORT_AMOUNT_ERROR = 'Too large'

export type CellDisplay = {
    text: string
    align: 'left' | 'right'
    isRed: boolean
    isError: boolean
}

// Integer cents divided by `divisor`, half away from zero. A float division near
// 2^53 can land on the wrong side of a half, so the remainder is split off with
// `%`, which is exact for integers.
function roundedQuotient(cents: number, divisor: number): number {
    const magnitude = Math.abs(cents)
    const remainder = magnitude % divisor
    const quotient = (magnitude - remainder) / divisor + (2 * remainder >= divisor ? 1 : 0)
    return cents < 0 ? -quotient : quotient
}

// The one rounding of an amount, to whole cents of the display unit. Null when the
// amount cannot be stored, so it cannot be shown either.
function displayUnits(cents: number, format: NumberFormat): number | null {
    if (!Number.isFinite(cents) || Math.abs(cents) > MAX_STORED_AMOUNT) return null
    const centsPerUnit = format.withoutCents ? 100 : 1
    const divisor = (format.divideBy1000 ? 1000 : 1) * centsPerUnit
    const rounded = Number.isInteger(cents)
        ? roundedQuotient(cents, divisor)
        : roundHalfAwayFromZero(cents / divisor)
    return rounded * centsPerUnit
}

function plainAmount(magnitude: number): string {
    return `${groupDigits(Math.floor(magnitude / 100))}.${String(magnitude % 100).padStart(2, '0')}`
}

function unitsText(
    units: number,
    currency: string,
    format: NumberFormat,
    withSymbol: boolean
): string {
    const magnitude = Math.abs(units)
    const amount = withSymbol ? formatMoney(magnitude, currency) : plainAmount(magnitude)
    const body = format.withoutCents ? amount.replace(/\.00(?=\D*$)/, '') : amount
    if (units >= 0) return body
    return format.negative === 'parens' ? `(${body})` : `-${body}`
}

export function formatReportAmount(
    cents: number,
    currency: string,
    format: NumberFormat,
    options: { withSymbol: boolean }
): string {
    const units = displayUnits(cents, format)
    if (units === null) return REPORT_AMOUNT_ERROR
    return unitsText(units, currency, format, options.withSymbol)
}

/** `value` is already a percent: 12.345 shows as 12.3%. A share of nothing shows nothing. */
export function formatReportPercent(value: number): string {
    if (!Number.isFinite(value)) return ''
    return `${(roundHalfAwayFromZero(value * 10) / 10).toFixed(1)}%`
}

export function formatNumber(value: number): string {
    if (Number.isSafeInteger(value)) return `${value < 0 ? '-' : ''}${groupDigits(Math.abs(value))}`
    return value.toLocaleString('en-US', { maximumFractionDigits: 6 })
}

const SYMBOL_ROWS: readonly RowKind[] = ['subtotal', 'computed', 'total']

/**
 * Group, subtotal, computed and total cells take the column's total type when it
 * has one; detail and opening cells have the column's own type (R5c).
 */
export function cellType(column: ResultColumn, row: ResultRow): FieldType {
    const isTotalRow = row.kind !== 'detail' && row.kind !== 'opening'
    return isTotalRow && column.totalType ? column.totalType : column.type
}

export function cellCurrency(column: ResultColumn, row: ResultRow): string {
    if (column.currency) return column.currency
    const cell = column.currencyCell ? row.cells[column.currencyCell] : null
    return typeof cell === 'string' ? cell : ''
}

const RIGHT_TYPES: readonly FieldType[] = ['money', 'number', 'percent']

export function isRightAligned(column: ResultColumn): boolean {
    return (
        RIGHT_TYPES.includes(column.type) || RIGHT_TYPES.includes(column.totalType ?? column.type)
    )
}

function plainDisplay(text: string, align: CellDisplay['align']): CellDisplay {
    return { text, align, isRed: false, isError: false }
}

function moneyDisplay(
    column: ResultColumn,
    row: ResultRow,
    cents: number,
    format: NumberFormat
): CellDisplay {
    const units = displayUnits(cents, format)
    if (units === null)
        return { text: REPORT_AMOUNT_ERROR, align: 'right', isRed: false, isError: true }
    return {
        text: unitsText(units, cellCurrency(column, row), format, SYMBOL_ROWS.includes(row.kind)),
        align: 'right',
        isRed: format.negativeRed && units < 0,
        isError: false,
    }
}

function dayText(value: string): string {
    const day = value.slice(0, 10)
    return isIsoDay(day) ? shortDay(day) : value
}

export const BLANK_LINK_LABEL = '(none)'

// A link cell shows its record's label, never the record's id: the label field's
// typed value, formatted as a cell of its type. A detail row's blank link (or a
// record the reader cannot see) reads (none); a group or total row with no value in
// the column shows nothing.
function linkDisplay(
    column: ResultColumn,
    labelCell: string,
    row: ResultRow,
    value: CellValue,
    format: NumberFormat
): CellDisplay {
    const label = row.cells[labelCell]
    if (label === null || label === undefined || label === '') {
        return plainDisplay(value === null && row.kind !== 'detail' ? '' : BLANK_LINK_LABEL, 'left')
    }
    const labelColumn: ResultColumn = {
        id: labelCell,
        label: column.label,
        type: column.labelType ?? 'text',
        currencyCell: column.labelCurrencyCell,
    }
    return { ...cellDisplay(labelColumn, row, label, format), align: 'left' }
}

export function cellDisplay(
    column: ResultColumn,
    row: ResultRow,
    value: CellValue | undefined,
    format: NumberFormat
): CellDisplay {
    const type = cellType(column, row)
    const align = RIGHT_TYPES.includes(type) ? 'right' : 'left'
    if (value === undefined) return plainDisplay('', align)
    if (column.labelCell && type === 'relation') {
        return linkDisplay(column, column.labelCell, row, value, format)
    }
    if (value === null) return plainDisplay('', align)
    if (type === 'money' && typeof value === 'number')
        return moneyDisplay(column, row, value, format)
    if (typeof value === 'number' && (type === 'percent' || column.format === 'percent')) {
        return plainDisplay(formatReportPercent(value), 'right')
    }
    if (typeof value === 'number') return plainDisplay(formatNumber(value), align)
    if (typeof value === 'boolean') return plainDisplay(value ? 'Yes' : 'No', align)
    if ((type === 'date' || type === 'datetime') && typeof value === 'string') {
        return plainDisplay(dayText(value), align)
    }
    if (type === 'select') {
        return plainDisplay(column.options?.find(o => o.value === value)?.label ?? value, align)
    }
    return plainDisplay(value, align)
}

const LABEL_ROW: ResultRow = { id: '', kind: 'detail', depth: 0, label: '', cells: {} }

// A typed label (a row's, a pivot heading's, a param option's) formatted as a cell
// of its type. An amount always shows its currency, so 300 EUR and 300 USD read apart.
export function typedLabelText(
    type: FieldType,
    value: CellValue,
    currency: string,
    format: NumberFormat
): string {
    if (isBucketLabelType(type) && (typeof value === 'string' || typeof value === 'number')) {
        return bucketWords(type, value)
    }
    if (type === 'money' && typeof value === 'number') {
        return formatReportAmount(value, currency, format, { withSymbol: true })
    }
    return cellDisplay({ id: '', label: '', type, currency }, LABEL_ROW, value, format).text
}

/**
 * A row's label as the table shows it. The engine types a label that is not text
 * (an amount, a date, a yes/no) and writes `label` as its plain text; a subtotal's
 * label is "Total " and its group's label.
 */
export function rowLabelText(row: ResultRow, format: NumberFormat): string {
    const { labelType, labelValue } = row
    if (!labelType || labelValue === undefined || labelValue === null) return row.label
    const text = typedLabelText(labelType, labelValue, row.labelCurrency ?? '', format)
    return row.kind === 'subtotal' ? `Total ${text}` : text
}

/**
 * A column's heading: its run heading (a day in the short day form), a pivot column
 * over typed values by its typed label, else its label.
 */
export function headingText(column: ResultColumn, format: NumberFormat): string {
    const { heading } = column
    if (heading && heading.value !== null) {
        return typedLabelText(heading.type, heading.value, '', format)
    }
    return columnName(column, format)
}

/**
 * A column's name, as Customize lists it: a run heading changes with the params
 * (a day), so the name stays the label; a pivot column is named by its typed label.
 */
export function columnName(column: ResultColumn, format: NumberFormat): string {
    const { pivot } = column
    if (!pivot?.labelType || pivot.labelValue === undefined || pivot.labelValue === null) {
        return column.label
    }
    return typedLabelText(pivot.labelType, pivot.labelValue, pivot.labelCurrency ?? '', format)
}

/**
 * The hidden list after Customize changes the columns it lists. A hidden id that the
 * current result does not have (a pivot value not in this run, a column of an earlier
 * design) stays hidden, so it does not come back on the next run.
 */
export function mergeHidden(
    hidden: readonly string[],
    listed: readonly { id: string; isShown: boolean }[]
): string[] {
    const listedIds = new Set(listed.map(column => column.id))
    return [
        ...hidden.filter(id => !listedIds.has(id)),
        ...listed.filter(column => !column.isShown).map(column => column.id),
    ]
}

export function shownColumns(
    columns: readonly ResultColumn[],
    hidden: readonly string[]
): ResultColumn[] {
    return columns.filter((column, index) => index === 0 || !hidden.includes(column.id))
}
