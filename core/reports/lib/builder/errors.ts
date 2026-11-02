import { extractValidationErrors } from '@tinycld/core/lib/errors'
import type { Problem } from '../types'

// The server answers a refused save or draft run with problems keyed by spec path
// (`columns.0.id`). The builder shows each under the section it is about, naming
// the part ("Column 3: …") (R17).

export type BuilderSection =
    | 'details'
    | 'columns'
    | 'filter'
    | 'group'
    | 'pivot'
    | 'layout'
    | 'arrange'

export function sectionOf(specPath: string): BuilderSection {
    const head = specPath.split('.')[0]
    // The row label is set under the columns.
    if (head === 'columns' || head === 'detail') return 'columns'
    if (head === 'filter' || head === 'params') return 'filter'
    if (head === 'groups' || head === 'sort') return 'group'
    if (head === 'pivot') return 'pivot'
    if (head === 'layout') return 'layout'
    if (head === 'display') return 'arrange'
    return 'details'
}

const PART_NAMES: Record<string, string> = {
    columns: 'Column',
    groups: 'Group',
    sort: 'Sort',
    layout: 'Layout item',
    params: 'Asked value',
    display: 'Arrangement',
}

export function describeProblem(problem: Problem): string {
    // A display problem is about one of its layouts: display.layouts.<n>.
    const parts = problem.path.split('.')
    const [head, index] = parts[0] === 'display' ? [parts[0], parts[2]] : parts
    const name = head ? PART_NAMES[head] : undefined
    if (name && index !== undefined && /^\d+$/.test(index))
        return `${name} ${Number(index) + 1}: ${problem.message}`
    if (head === 'filter') return `Filter: ${problem.message}`
    if (head === 'pivot') return `Pivot: ${problem.message}`
    if (head === 'detail') return `Row label: ${problem.message}`
    return problem.message
}

export function problemsFromError(error: unknown): Problem[] {
    const fields = extractValidationErrors(error) ?? {}
    return Object.entries(fields)
        .map(([path, message]) => ({ path, code: '', message }))
        .sort((a, b) => a.path.localeCompare(b.path))
}

export function problemsFor(section: BuilderSection, problems: readonly Problem[]): Problem[] {
    return problems.filter(problem => sectionOf(problem.path) === section)
}
