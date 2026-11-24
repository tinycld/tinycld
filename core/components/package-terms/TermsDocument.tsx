import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { MarkdownRenderer } from '@tinycld/core/components/help/MarkdownRenderer'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { Text, View } from 'react-native'

/**
 * The terms text of one package, as the installed server holds it
 * (pkg_terms, written at boot). Any signed-in user may read it.
 */
export function TermsDocument({ slug }: { slug: string }) {
    const [pkgTerms] = useStore('pkg_terms')
    const { data: rows = [], isReady } = useLiveQuery({
        queryKey: ['core', 'pkg-terms', 'text', slug],
        query: query => query.from({ t: pkgTerms }).where(({ t }) => eq(t.pkg, slug)),
    })
    const text = rows[0]

    if (!isReady) return null
    if (!text) {
        return (
            <Text testID="pkg-terms-missing" className="text-sm text-muted-foreground">
                The text of these terms is not available. Try again later.
            </Text>
        )
    }
    return (
        <View testID="pkg-terms-body" className="gap-2">
            <Text className="text-xs text-muted-foreground">Version {text.version}</Text>
            {/* The author's own text: no shortcut glyph translation, no help table layout. */}
            <MarkdownRenderer
                body={text.body}
                translateModifierKeys={false}
                shortcutTableHeuristic={false}
            />
        </View>
    )
}
