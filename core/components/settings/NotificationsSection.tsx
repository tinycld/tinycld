import { SectionCard } from '@tinycld/core/components/settings/SectionCard'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import {
    type MailNotifyMode,
    type NotificationPreferences,
    useNotificationPreferences,
} from '@tinycld/core/lib/use-notification-preferences'
import { usePushSubscription } from '@tinycld/core/lib/use-push-subscription'
import { Switch } from '@tinycld/core/ui/switch'
import { ActivityIndicator, Platform, Pressable, Text, View } from 'react-native'

export function NotificationsSection() {
    const { isSupported, isConfigured, isSubscribed, subscribe, unsubscribe, isPending } =
        usePushSubscription()

    const handlePushToggle = () => {
        if (isSubscribed) {
            unsubscribe()
        } else {
            subscribe()
        }
    }

    return (
        <View className="gap-3">
            <PushToggle
                isSupported={Platform.OS === 'web' && isSupported}
                isConfigured={isConfigured}
                isNative={Platform.OS !== 'web'}
                isSubscribed={isSubscribed}
                isPending={isPending}
                onToggle={handlePushToggle}
            />
            <NotificationTypeToggles />
        </View>
    )
}

function PushToggle({
    isSupported,
    isConfigured,
    isNative,
    isSubscribed,
    isPending,
    onToggle,
}: {
    isSupported: boolean
    isConfigured: boolean
    isNative: boolean
    isSubscribed: boolean
    isPending: boolean
    onToggle: () => void
}) {
    if (isNative) {
        return (
            <SectionCard>
                <Text className="text-foreground text-base">
                    Push notifications are managed by your device settings.
                </Text>
            </SectionCard>
        )
    }

    if (!isSupported) {
        return (
            <SectionCard>
                <Text className="text-muted-foreground text-[13px]">
                    Your browser does not support push notifications.
                </Text>
            </SectionCard>
        )
    }

    // Supported by the browser but the server has no VAPID keypair. Say so
    // rather than showing a switch that can only fail.
    if (!isConfigured) {
        return (
            <SectionCard>
                <Text className="text-muted-foreground text-[13px]">
                    Push notifications are not set up on this server. An administrator can enable
                    them under Settings → System → Web Push.
                </Text>
            </SectionCard>
        )
    }

    return (
        <SectionCard>
            <Pressable onPress={onToggle} disabled={isPending}>
                <View className="flex-row items-center gap-3">
                    <View className="flex-1 gap-0.5">
                        <Text className="text-foreground text-base font-semibold">
                            Browser Push Notifications
                        </Text>
                        <Text className="text-muted-foreground text-[13px]">
                            Receive calendar reminders even when the browser tab is closed.
                        </Text>
                    </View>
                    {isPending ? (
                        <ActivityIndicator size="small" />
                    ) : (
                        <Switch value={isSubscribed} onValueChange={onToggle} />
                    )}
                </View>
            </Pressable>
        </SectionCard>
    )
}

const NOTIF_GROUPS: {
    label: string
    types: { key: keyof NotificationPreferences; label: string }[]
}[] = [
    {
        label: 'Calendar',
        types: [
            { key: 'calendar_reminder', label: 'Event reminders' },
            { key: 'calendar_invite', label: 'Calendar invites' },
            { key: 'calendar_subscription_error', label: 'Subscription sync errors' },
        ],
    },
    {
        label: 'Mail',
        types: [{ key: 'mail_new_message', label: 'New messages' }],
    },
    {
        label: 'Drive',
        types: [{ key: 'drive_file_shared', label: 'Files shared with you' }],
    },
    {
        label: 'Boards',
        types: [
            { key: 'boards_mention', label: 'Mentions on a card' },
            { key: 'boards_assigned', label: 'Cards assigned to you' },
            { key: 'boards_reply', label: 'Replies to your comments' },
            { key: 'boards_reaction', label: 'Reactions to your comments' },
            { key: 'boards_watched', label: 'Changes to cards you watch' },
            { key: 'boards_due', label: 'Due-date reminders' },
            { key: 'boards_sprint', label: 'Sprint starts and completes' },
        ],
    },
    {
        label: 'General',
        types: [
            { key: 'org_invite', label: 'Organization invites' },
            { key: 'system_error', label: 'System errors' },
        ],
    },
]

const MAIL_MODE_OPTIONS: { value: MailNotifyMode; label: string; description: string }[] = [
    {
        value: 'batched',
        label: 'All messages (batched)',
        description: 'Notify for all incoming messages, batched every 2 minutes',
    },
    {
        value: 'important_only',
        label: 'Important only',
        description: 'Only notify for messages from your contacts',
    },
]

function NotificationTypeToggles() {
    const { prefs, setTypeEnabled, mailMode, setMailMode } = useNotificationPreferences()

    return (
        <SectionCard>
            <View className="gap-4">
                {NOTIF_GROUPS.map(group => (
                    <View key={group.label} className="gap-1.5">
                        <Text
                            className="text-muted-foreground text-[13px] font-semibold uppercase"
                            style={{ letterSpacing: 0.5 }}
                        >
                            {group.label}
                        </Text>
                        {group.types.map(type => (
                            <NotifTypeRow
                                key={type.key}
                                label={type.label}
                                enabled={prefs[type.key]}
                                onToggle={val => setTypeEnabled(type.key, val)}
                            />
                        ))}
                        <MailModeSelector
                            isVisible={group.label === 'Mail' && prefs.mail_new_message}
                            mailMode={mailMode}
                            onSelect={setMailMode}
                        />
                    </View>
                ))}
            </View>
        </SectionCard>
    )
}

function NotifTypeRow({
    label,
    enabled,
    onToggle,
}: {
    label: string
    enabled: boolean
    onToggle: (val: boolean) => void
}) {
    return (
        <View className="flex-row items-center justify-between py-1.5">
            <Text className="text-foreground text-[15px]">{label}</Text>
            <Switch value={enabled} onValueChange={onToggle} />
        </View>
    )
}

function MailModeSelector({
    isVisible,
    mailMode,
    onSelect,
}: {
    isVisible: boolean
    mailMode: MailNotifyMode
    onSelect: (mode: MailNotifyMode) => void
}) {
    const primaryColor = useThemeColor('primary')

    if (!isVisible) return null

    return (
        <View className="gap-1 ml-2">
            {MAIL_MODE_OPTIONS.map(opt => (
                <Pressable
                    key={opt.value}
                    onPress={() => onSelect(opt.value)}
                    className="flex-row items-center py-1.5 gap-2"
                >
                    <View
                        className={`w-4 h-4 rounded-full border-2 items-center justify-center ${mailMode === opt.value ? 'border-primary' : 'border-muted-foreground'}`}
                    >
                        <RadioDot isVisible={mailMode === opt.value} color={primaryColor} />
                    </View>
                    <View>
                        <Text className="text-foreground text-sm">{opt.label}</Text>
                        <Text className="text-muted-foreground text-xs">{opt.description}</Text>
                    </View>
                </Pressable>
            ))}
        </View>
    )
}

function RadioDot({ isVisible, color }: { isVisible: boolean; color: string }) {
    if (!isVisible) return null
    return (
        <View
            style={{
                width: 8,
                height: 8,
                borderRadius: 4,
                backgroundColor: color,
            }}
        />
    )
}
