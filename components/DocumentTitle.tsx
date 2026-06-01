import Head from 'expo-router/head'
import { Platform } from 'react-native'
import { getCoreConfigOptional } from '../lib/core-config'
import { useOrgInfo } from '../lib/use-org-info'

interface DocumentTitleProps {
    /**
     * Leaf segment — typically the most specific thing on screen
     * (document name, mail subject, settings section). Null / undefined
     * / blank drops the segment entirely; the tab then shows just the
     * brand + optional org + optional pkg.
     */
    title?: string | null
    /**
     * Middle segment — typically the package or area name ("Mail",
     * "Settings", "Help"). Omit on screens that already say what they
     * are (e.g. the org root).
     */
    pkg?: string
    /**
     * When false, suppresses the org segment even if a current org is
     * available. Use on pre-auth screens (Connect, Setup), or on the
     * org root where `title` itself IS the org name.
     */
    includeOrg?: boolean
    /**
     * Truncates the leaf with an ellipsis past this many characters.
     * Only the leaf is truncated — pkg and org are assumed short.
     * Defaults to 40; pass Infinity to disable.
     */
    maxDetailChars?: number
}

/**
 * Sets the browser tab title on web. On iOS, forwards into the
 * NSUserActivity bridge so the route becomes addressable by Spotlight
 * and Handoff. Android renders nothing (expo-router/head is a no-op).
 *
 * Final tab string is "<brand>[: <org>][ — <pkg>][ — <title>]", with
 * empty segments dropped. When no segments are present at all the tab
 * collapses to "<brand>".
 *
 * The brand comes from CoreConfig.brandName (which the app derives
 * from app.json's expo.name), so a fork rebrands the whole app by
 * editing app.json alone.
 */
export function DocumentTitle({
    title,
    pkg,
    includeOrg = true,
    maxDetailChars = 40,
}: DocumentTitleProps) {
    const brand = getCoreConfigOptional()?.brandName ?? 'TinyCld'
    // useOrgInfo is safe outside OrgSlugProvider: useOrgSlug() returns ''
    // when there's no provider and no URL param, the live query matches
    // no rows, and org collapses to null. So pre-auth screens that mount
    // <DocumentTitle> don't need any guard — they just get no org segment.
    const { org } = useOrgInfo()

    const segments: string[] = []
    if (includeOrg && org?.name?.trim()) segments.push(org.name.trim())
    if (pkg?.trim()) segments.push(pkg.trim())
    const leaf = typeof title === 'string' ? title.trim() : ''
    if (leaf) {
        const truncated =
            leaf.length > maxDetailChars ? `${leaf.slice(0, maxDetailChars - 1)}…` : leaf
        segments.push(truncated)
    }

    const text = segments.length > 0 ? `${brand}: ${segments.join(' — ')}` : brand
    if (Platform.OS !== 'web') return null
    return (
        <Head>
            <title>{text}</title>
        </Head>
    )
}
