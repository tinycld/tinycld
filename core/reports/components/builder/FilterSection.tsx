import { useWatch } from '@tinycld/core/ui/form'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import type { Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'
import { ConditionTree } from './ConditionTree'

// The report's filter: which rows the report reads. The form's refusals (a value to
// enter) show above the conditions; the server's show in the section's problems.
export function FilterSection({
    builder,
    problems,
}: {
    builder: ReportBuilder
    problems: Problem[]
}) {
    const root = useWatch({ control: builder.form.control, name: 'filter' })
    const error = builder.form.getFieldState('filter', builder.form.formState).error?.message
    return (
        <BuilderSection title="Filter" section="filter" problems={problems}>
            <ConditionTree
                builder={builder}
                root={root}
                onChange={builder.setFilter}
                baseKey={builder.base.key}
                testID="report-builder-filter"
                error={error}
            />
        </BuilderSection>
    )
}
