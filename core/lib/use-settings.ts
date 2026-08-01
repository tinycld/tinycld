import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useStore } from '@tinycld/core/lib/pocketbase'

export function useSettings(app: string) {
    const [settingsCollection] = useStore('settings')

    const { data: settings } = useLiveQuery(
        query =>
            query
                .from({ settings: settingsCollection })
                .where(({ settings }) => eq(settings.app, app)),
        [app]
    )

    return settings ?? []
}
