import { useWindowWidth } from '@tinycld/core/components/workspace/useBreakpoint'
import { useMemo, useRef, useState } from 'react'
import { type ColumnMeasures, measureColumns, measureKey } from '../lib/column-measure'
import { type LineLayout, tableLines } from '../lib/table-lines'
import type { ReportViewModel } from './useReportView'
import { useStable } from './useStable'

export type TableLayoutModel = Pick<
    ReportViewModel,
    'columns' | 'rows' | 'display' | 'layouts' | 'rowLevels' | 'run'
>

export interface TableLayout {
    layout: LineLayout
    /** False until the rows' width is measured: the table is drawn hidden until then. */
    isMeasured: boolean
    onRowWidth(width: number): void
}

// Which run's rows are shown: its design and its resolved params. It changes when
// a run of a new design or new params lands; not while that run loads behind the
// earlier rows, not when a live re-run of the same design and params answers, and
// not on a group fold, a page load or a display-only change.
export function shownRunKey(model: Pick<ReportViewModel, 'run'>): string {
    return JSON.stringify([model.run.shownRowsKey ?? '', model.run.result?.params ?? []])
}

// The measure taken from the rows shown when the key first appears, kept while the
// key stays (D1): a later page, a fold or a live re-run never moves a column. It is
// computed in render and kept in a ref by its key, with no state and no effect: the
// measure is derived from the rows, only its moment is kept. A report whose groups
// start closed is measured once more when its first detail rows are shown, so the
// text columns get their content's width then, not their heading's. That happens
// once per key: folding every group again hides the details but never measures.
function useRunMeasures(model: TableLayoutModel): ColumnMeasures {
    const levels = model.rowLevels
    const key = measureKey({
        runKey: shownRunKey(model),
        format: model.display,
        columns: model.columns,
        levels,
    })
    const hasDetail = model.rows.some(item => item.row.kind === 'detail')
    const kept = useRef<{ key: string; hasDetail: boolean; measures: ColumnMeasures } | null>(null)
    const isNewKey = kept.current?.key !== key
    const isFirstDetail = hasDetail && kept.current?.hasDetail === false
    if (kept.current === null || isNewKey || isFirstDetail) {
        const measures = measureColumns({
            columns: model.columns,
            rows: model.rows,
            format: model.display,
            levels,
        })
        kept.current = { key, hasDetail, measures }
    }
    return kept.current.measures
}

/**
 * The lines of a report table at the width its rows really get. The width is
 * measured by the table's probe (a builder drawer or the Customize panel narrows
 * it); until then the window's width stands in and the table is drawn hidden.
 */
export function useTableLayout(model: TableLayoutModel): TableLayout {
    const windowWidth = useWindowWidth()
    const [rowWidth, setRowWidth] = useState<number>()
    const measures = useRunMeasures(model)
    const columns = useStable(model.columns)
    const levels = useStable(model.rowLevels)
    const { layouts } = model
    const width = rowWidth ?? windowWidth
    const layout = useMemo(
        () => tableLines({ columns, width, layouts, levels, measures }),
        [columns, width, layouts, levels, measures]
    )
    return { layout, isMeasured: rowWidth !== undefined, onRowWidth: setRowWidth }
}
