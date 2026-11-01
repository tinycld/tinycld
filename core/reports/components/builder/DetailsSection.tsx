import { TextAreaInput, TextInput } from '@tinycld/core/ui/form'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import type { Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'

// Open at start in both modes: the title is what an edit most often changes.
export function DetailsSection({
    builder,
    problems,
}: {
    builder: ReportBuilder
    problems: Problem[]
}) {
    const { control } = builder.form
    return (
        <BuilderSection title="Details" section="details" problems={problems} isOpenAtStart>
            <TextInput
                name="title"
                control={control}
                label="Title"
                testID="report-builder-title"
                maxLength={200}
            />
            <TextAreaInput
                name="description"
                control={control}
                label="Description"
                testID="report-builder-description"
            />
        </BuilderSection>
    )
}
