import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { Slot } from 'expo-router'

// A Slot, as help and settings use: one report screen shows at a time, and the
// router history (not a stack of mounted screens) takes Back to the hub.
export default function ReportsLayout() {
    return (
        <>
            <DocumentTitle pkg="Reports" />
            <Slot />
        </>
    )
}
