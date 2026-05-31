import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { Slot } from 'expo-router'

export default function SettingsLayout() {
    return (
        <>
            <DocumentTitle pkg="Settings" />
            <Slot />
        </>
    )
}
