import { extractValidationErrors } from '@tinycld/core/lib/errors'
import type { Problem } from '../types'

// The server answers a refused save or draft run with problems keyed by spec path
// (`columns.0.id`). The builder shows each under the section it is about, naming
// the part ("Column 3: …") (R17).

export type BuilderSection = 'details' | 'columns' | 'filter' | 'group' | 'pivot' | 'layout'

export function sectionOf(specPath: string): BuilderSection {
    const head = specPath.split('.')[0]
    if (head === 'columns') return 'columns'
    if (head === 'filter' || head === 'params') return 'filter'
    if (head === 'groups' || head === 'sort') return 'group'
    if (head === 'pivot') return 'pivot'
    if (head === 'layout') return 'layout'
    return 'details'
}

const PART_NAMES: Record<string, string> = {
    columns: 'Column',
    groups: 'Group',
    sort: 'Sort',
    layout: 'Layout item',
    params: 'Asked value',
}

export function describeProblem(problem: Problem): string {
    const [head, index] = problem.path.split('.')
    const name = head ? PART_NAMES[head] : undefined
    if (name && index !== undefined && /^\d+$/.test(index))
        return `${name} ${Number(index) + 1}: ${problem.message}`
    if (head === 'filter') return `Filter: ${problem.message}`
    if (head === 'pivot') return `Pivot: ${problem.message}`
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
