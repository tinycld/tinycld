import { useUserPreference } from '@tinycld/core/lib/use-user-preference'

// Each key is a notification TYPE string, matching the `Type` the server sends
// in NotifyParams — server/notify/notify.go looks the type up in this map by
// name and skips the send when it is false. A type missing from the stored
// preferences is NOT muted (the server defaults to sending), so adding a key
// here is what makes an existing notification mutable, not what enables it.
export interface NotificationPreferences {
    calendar_reminder: boolean
    calendar_invite: boolean
    calendar_subscription_error: boolean
    mail_new_message: boolean
    drive_file_shared: boolean
    org_invite: boolean
    system_error: boolean
    /**
     * Being @mentioned in a document comment (text, calc) — the type
     * server/notify/comment_mentions.go sends. This was MISSING, which meant
     * mention notifications could not be muted at all; the omission predates
     * cards and affected text/calc too.
     */
    comment_mention: boolean
    /**
     * Being @mentioned on a card — its own switch rather than sharing
     * `comment_mention`, because the two are different enough in cadence to
     * want muting separately: a board is chattier than a document, and someone
     * who wants card noise off usually still wants a doc mention. Sent by
     * cards' description flush hook (server/description_mentions.go) and by
     * the comment path for `cards_comments`.
     */
    cards_mention: boolean
}

const DEFAULT_PREFS: NotificationPreferences = {
    calendar_reminder: true,
    calendar_invite: true,
    calendar_subscription_error: true,
    mail_new_message: true,
    drive_file_shared: true,
    org_invite: true,
    system_error: true,
    comment_mention: true,
    cards_mention: true,
}

export type MailNotifyMode = 'batched' | 'important_only'

export function useNotificationPreferences() {
    const [prefs, setPrefs] = useUserPreference('notifications', 'preferences', DEFAULT_PREFS)
    const [mailMode, setMailMode] = useUserPreference<MailNotifyMode>(
        'notifications',
        'mail_notify_mode',
        'batched'
    )

    const setTypeEnabled = (type: keyof NotificationPreferences, enabled: boolean) => {
        setPrefs({ ...prefs, [type]: enabled })
    }

    return {
        prefs,
        setPrefs,
        setTypeEnabled,
        mailMode,
        setMailMode,
    }
}
