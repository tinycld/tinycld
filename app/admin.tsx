import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { SetupPage } from '@tinycld/core/components/setup/SetupPage'
import { useLocalSearchParams } from 'expo-router'

export default function Admin() {
    const { token } = useLocalSearchParams<{ token?: string }>()
    return (
        <>
            <DocumentTitle title="Admin" includeOrg={false} />
            <SetupPage token={token} />
        </>
    )
}
