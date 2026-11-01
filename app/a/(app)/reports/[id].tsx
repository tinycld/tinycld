import { ReportScreen } from '@tinycld/core/reports/screens/ReportScreen'
import { useLocalSearchParams } from 'expo-router'

export default function ReportRoute() {
    const { id } = useLocalSearchParams<{ id: string }>()
    return <ReportScreen reportId={id} />
}
