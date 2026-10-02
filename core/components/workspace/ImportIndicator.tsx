import { appHref } from '@tinycld/core/lib/org-routes'
import { useTakeoutImportStore } from '@tinycld/core/lib/stores/takeout-import-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { type Href, Link, usePathname } from 'expo-router'
import { ActivityIndicator } from 'react-native'

export function ImportIndicator() {
    const phase = useTakeoutImportStore(s => s.phase)
    const href = useTakeoutImportStore(s => s.progressHref) ?? appHref('settings')
    const pathname = usePathname()
    const railText = useThemeColor('rail-text')

    if (phase !== 'importing') return null
    // Already on the import screen, which shows its own progress.
    if (pathname && href.endsWith(pathname)) return null

    return (
        <Link
            href={href as Href}
            style={{
                width: 44,
                height: 44,
                borderRadius: 12,
                justifyContent: 'center',
                alignItems: 'center',
                display: 'flex',
            }}
            aria-label="Import in progress"
        >
            <ActivityIndicator size="small" color={railText} />
        </Link>
    )
}
