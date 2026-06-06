import { useAuth } from '@tinycld/core/lib/auth'
import { useCurrentUserOrg } from '@tinycld/core/lib/use-current-user-org'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
import { useOrgSlug } from '@tinycld/core/lib/use-org-slug'
import { router } from 'expo-router'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import { LeaveOrgFlow } from './LeaveOrgFlow'

// Self-leave entry point. Shown in personal settings as a less-destructive
// alternative to deleting the whole account when the user just wants to
// leave the current org. Uses the same LeaveOrgFlow as the admin path.
//
// Uses the existing useCurrentUserOrg helper (which itself does live-query
// resolution of orgSlug→user_org). This avoids reimplementing the same
// query and keeps the org-scoped lookup consistent with the rest of the app.
export function LeaveOrgSection() {
    const orgSlug = useOrgSlug()
    const { org } = useOrgInfo()
    const orgName = org?.name ?? 'this org'
    const { logout } = useAuth()
    const userOrg = useCurrentUserOrg(orgSlug)
    const [open, setOpen] = useState(false)

    if (!userOrg) return null

    return (
        <View className="gap-3">
            <Text className="text-foreground text-xl font-bold">Leave organization</Text>
            <View className="rounded-xl border border-border bg-surface-secondary p-4 gap-2">
                <Text className="text-foreground text-base font-semibold">Leave {orgName}</Text>
                <Text className="text-muted-foreground text-[13px]">
                    Remove yourself from this org. You'll choose what happens to anything you
                    created — reassign to another member or delete it.
                </Text>
                <Pressable
                    onPress={() => setOpen(true)}
                    className="self-start rounded-lg mt-1 px-3 py-2 border border-border"
                >
                    <Text className="text-foreground font-semibold">Leave {orgName}</Text>
                </Pressable>
            </View>
            <LeaveOrgFlow
                isVisible={open}
                onClose={() => setOpen(false)}
                onSuccess={result => {
                    setOpen(false)
                    // If anonymize fired (last org gone) the session is now
                    // dead — log out + back to connect screen. Otherwise drop
                    // the user back to a neutral landing so they pick another org.
                    if (result.user_anonymized) {
                        logout()
                        router.replace('/connect')
                    } else {
                        router.replace('/')
                    }
                }}
                userOrgId={userOrg.id}
                mode="self"
            />
        </View>
    )
}
