import { useAuth } from '@tinycld/core/lib/auth'
import { CONNECT_HREF } from '@tinycld/core/lib/org-routes'
import { router } from 'expo-router'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import { DisableAccountFlow } from './DisableAccountFlow'

// Reversible suspension, offered alongside (and above) the irreversible
// DeleteAccountSection in personal settings.
//
// Single-org: this replaces LeaveOrgSection. "Leave the organization" is
// meaningless when the deployment IS the org, so the softer option is to
// suspend the account rather than to leave something.
export function DisableAccountSection() {
    const { user, logout } = useAuth()
    const [isOpen, setIsOpen] = useState(false)

    if (!user?.id) return null

    return (
        <View className="gap-3">
            <Text className="text-xl font-bold text-foreground">Account access</Text>
            <View className="rounded-xl border border-border bg-surface-secondary p-4 gap-2">
                <Text className="text-base font-semibold text-foreground">Disable account</Text>
                <Text className="text-[13px] text-muted-foreground">
                    Sign out everywhere and block sign-in and shared-file access until an
                    administrator restores it. Nothing is deleted.
                </Text>
                <Pressable
                    testID="disable-account-open"
                    onPress={() => setIsOpen(true)}
                    className="self-start rounded-lg mt-1 px-3 py-2 border border-border"
                >
                    <Text className="text-foreground font-semibold">Disable my account</Text>
                </Pressable>
            </View>
            <DisableAccountFlow
                isVisible={isOpen}
                onClose={() => setIsOpen(false)}
                onSuccess={() => {
                    setIsOpen(false)
                    // The server rotated the token key, so this session is
                    // already dead — clear it locally rather than leaving a
                    // stale token in storage.
                    logout()
                    router.replace(CONNECT_HREF)
                }}
                email={user.email}
            />
        </View>
    )
}
