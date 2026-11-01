import { Text, View } from 'react-native'

function Line({
    text,
    testID,
    className,
}: {
    text: string | null
    testID: string
    className: string
}) {
    if (!text) return null
    return (
        <Text testID={testID} className={className}>
            {text}
        </Text>
    )
}

interface ReportDocumentHeaderProps {
    orgName: string
    title: string
    periodWords: string | null
    isThousands: boolean
}

// The finished-document strip above the table: who, what and which period.
export function ReportDocumentHeader({
    orgName,
    title,
    periodWords,
    isThousands,
}: ReportDocumentHeaderProps) {
    const businessName = orgName.trim() || null
    const thousandsNote = isThousands ? 'Amounts in thousands' : null
    return (
        <View
            testID="report-document-header"
            className="gap-0.5 border-y border-border bg-card px-4 py-3"
        >
            <Line
                text={businessName}
                testID="report-business-name"
                className="text-xs text-muted-foreground"
            />
            <Text testID="report-document-title" className="text-lg font-semibold text-foreground">
                {title}
            </Text>
            <Line
                text={periodWords}
                testID="report-document-period"
                className="text-sm text-muted-foreground"
            />
            <Line
                text={thousandsNote}
                testID="report-thousands-note"
                className="text-sm text-muted-foreground"
            />
        </View>
    )
}
