import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { Slot } from 'expo-router'

export default function HelpLayout() {
    return (
        <>
            <DocumentTitle pkg="Help" />
            <Slot />
        </>
    )
}
