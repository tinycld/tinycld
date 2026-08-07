import { usePathname } from 'expo-router'
import { searchPackages } from './registry'

/**
 * The slug of the package the user is currently viewing, or null outside one.
 *
 * Derived from the path rather than stored, because the route IS the source of
 * truth — a separate store would drift on back/forward navigation. Only
 * packages that declare `search` can be seeded, so a match here is always a
 * valid chip.
 */
export function useActivePackageSlug(): string | null {
    const pathname = usePathname()
    const segments = pathname.split('/').filter(Boolean)
    for (const segment of segments) {
        if (searchPackages.some(p => p.slug === segment)) return segment
    }
    return null
}
