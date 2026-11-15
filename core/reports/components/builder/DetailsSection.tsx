import { TextAreaInput, TextInput, useWatch } from '@tinycld/core/ui/form'
import { Text } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import type { Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'

// An open link is set in the spec only (Q10): the builder keeps it and says what it does.
function DetailOpenNote({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text testID="report-builder-detail-open" className="text-sm text-muted-foreground">
            Rows open their record
        </Text>
    )
}

// Open at start in both modes: the title is what an edit most often changes.
export function DetailsSection({
    builder,
    problems,
}: {
    builder: ReportBuilder
    problems: Problem[]
}) {
    const { control } = builder.form
    const detailOpen = useWatch({ control, name: 'detailOpen' })
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
            <DetailOpenNote isVisible={detailOpen !== undefined} />
        </BuilderSection>
    )
}
