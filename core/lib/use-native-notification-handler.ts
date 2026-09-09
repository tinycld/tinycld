import { log } from '@tinycld/core/lib/logger'
import { router } from 'expo-router'
import { useEffect } from 'react'
import { Platform } from 'react-native'

/**
 * Foreground presentation + tap routing for native OS notifications.
 *
 * Two gaps this closes, both native-only:
 *
 *  1. Expo suppresses a notification that arrives while the app is in the
 *     FOREGROUND unless a handler opts in. Without this, a push that landed
 *     while the user had the app open simply never appeared.
 *  2. Tapping a notification did nothing beyond opening the app. The server
 *     already ships a deep-link target in `data.url` (see notify.go's
 *     sendExpoPush, which sets data:{url}), so the payload was there —
 *     nothing consumed it.
 *
 * Web needs neither: the service worker (public/sw.js) handles both display
 * and `notificationclick` routing itself.
 */
export function useNativeNotificationHandler() {
    useEffect(() => {
        if (Platform.OS === 'web') return

        let disposed = false
        // Held so cleanup can remove a listener that may be registered after
        // this effect is torn down (the import below is async).
        let subscription: { remove: () => void } | null = null

        const routeTo = (data: unknown) => {
            const url = (data as { url?: unknown } | undefined)?.url
            if (typeof url !== 'string' || url === '') return
            router.push(url as never)
        }

        import('expo-notifications')
            .then(Notifications => {
                if (disposed) return

                Notifications.setNotificationHandler({
                    handleNotification: async () => ({
                        shouldShowBanner: true,
                        shouldShowList: true,
                        shouldPlaySound: true,
                        shouldSetBadge: true,
                    }),
                })

                subscription = Notifications.addNotificationResponseReceivedListener(response => {
                    routeTo(response.notification.request.content.data)
                })

                // A tap that COLD-STARTS the app is delivered as the last
                // response rather than through the listener above, so it has to
                // be read once explicitly or the deep link is lost on launch.
                Notifications.getLastNotificationResponseAsync()
                    .then(response => {
                        if (disposed || !response) return
                        routeTo(response.notification.request.content.data)
                    })
                    .catch(err => {
                        log.warn('core.notifications', 'last response read failed', { err })
                    })
            })
            .catch(err => {
                log.warn('core.notifications', 'handler setup failed', { err })
            })

        return () => {
            disposed = true
            subscription?.remove()
        }
    }, [])
}
