import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import type { ReactNode } from 'react'
import { useEffect, useMemo, useState } from 'react'
import {
    Image,
    type ImageStyle,
    Linking,
    Platform,
    Text,
    type TextStyle,
    View,
    type ViewStyle,
} from 'react-native'
import Markdown, { type MarkedStyles, Renderer } from 'react-native-marked'
import { openHelp } from '../../lib/help/open-help'
import { parseHelpTopicId } from '../../lib/help/types'

interface Props {
    body: string
    /**
     * Intercepts a link press. Return true to signal "handled"; anything else
     * falls through to the default `Linking.openURL`.
     *
     * Exists so a consumer outside the help system can render markdown without
     * the `help://` scheme — and, more importantly, without this module's
     * import edge to the help store. Help passes `openHelpLink`.
     */
    onLinkPress?: LinkPressHandler
    /**
     * Swap ⌘/⇧/⌥ for Ctrl/Shift/Alt on non-Mac platforms. Right for help
     * topics, which are authored once with Mac glyphs; wrong for user-authored
     * prose, where a typed ⌘ must survive verbatim.
     */
    translateModifierKeys?: boolean
    /** Give shortcut-shaped tables a 20/80 column split. Help-specific. */
    shortcutTableHeuristic?: boolean
    /**
     * Rewrite an image URI before it renders. Exists for protected PocketBase
     * files: the stored src is tokenless (see lib/editor/rich/authed-image.ts)
     * and the bytes 404 without a fresh `?token=`, which only the consumer can
     * supply. Pass a stable reference — identity keys the renderer cache.
     */
    transformImageUri?: ImageUriTransform
    /**
     * Cap rendered images to this height, letterboxed with `contain`. Exists
     * for compact surfaces — a card comment shouldn't be dominated by a
     * full-width screenshot the way a description or help topic can be.
     * Undefined keeps the library's own sizing.
     */
    imageMaxHeight?: number
}

const HELP_SCHEME = 'help://'

/**
 * A link interceptor. Returning true means "handled, don't open the URL";
 * returning nothing falls through to the default.
 */
export type LinkPressHandler = (href: string) => boolean | undefined

/** An image-src rewriter; returns the URI to actually fetch. */
export type ImageUriTransform = (uri: string) => string

// Source markdown is authored with ⌘ (and ⇧ for Shift) because the
// Mac glyphs are unambiguous and look right inline. On Windows/Linux/
// Android the renderer's text-token override swaps them for the
// platform-correct equivalents at render time — see HelpRenderer.text
// below. Doing it in the renderer (vs. preprocessing the body string)
// means inline `code spans` with a literal ⌘ glyph stay verbatim, and
// we don't construct a parallel translated string per render.
function isMacLike(): boolean {
    if (Platform.OS === 'ios') return true
    if (Platform.OS === 'macos') return true
    if (Platform.OS === 'web' && typeof navigator !== 'undefined') {
        const ua = navigator.userAgent ?? ''
        return /Mac|iPhone|iPad|iPod/.test(ua)
    }
    return false
}

function translateModifierKeys(value: string): string {
    return value.replace(/⌘/g, 'Ctrl').replace(/⇧/g, 'Shift').replace(/⌥/g, 'Alt')
}

/**
 * The `help://` scheme handler. Exported so HelpTopicView can pass it back in
 * as `onLinkPress` — keeping the help-specific behavior opt-in rather than
 * baked into every consumer.
 */
export function openHelpLink(href: string): boolean {
    if (!href.startsWith(HELP_SCHEME)) return false
    const id = href.slice(HELP_SCHEME.length)
    if (!parseHelpTopicId(id)) return false
    openHelp(id as `${string}:${string}`)
    return true
}

function handleLinkPress(href: string, onLinkPress?: LinkPressHandler) {
    if (onLinkPress?.(href) === true) return
    Linking.openURL(href).catch(() => {})
}

interface RendererOptions {
    translateKeys: boolean
    shortcutTables: boolean
    onLinkPress?: LinkPressHandler
    transformImageUri?: ImageUriTransform
    imageMaxHeight?: number
}

/**
 * An image scaled DOWN to fit a height cap, never up past its natural size.
 * The intrinsic dimensions arrive async (Image.getSize), so until they do the
 * box reserves the cap square — a brief placeholder beats a layout jump when
 * the ratio lands. maxWidth keeps a very wide image inside the column, with
 * `contain` absorbing the difference.
 */
function CappedImage({ uri, alt, maxHeight }: { uri: string; alt?: string; maxHeight: number }) {
    const [size, setSize] = useState<{ width: number; height: number } | null>(null)

    useEffect(() => {
        let isLive = true
        Image.getSize(
            uri,
            (width, height) => {
                if (isLive && width > 0 && height > 0) setSize({ width, height })
            },
            () => {}
        )
        return () => {
            isLive = false
        }
    }, [uri])

    const height = size ? Math.min(size.height, maxHeight) : maxHeight
    const aspectRatio = size ? size.width / size.height : 1

    return (
        <Image
            source={{ uri }}
            accessibilityRole="image"
            accessibilityLabel={alt || 'Image'}
            resizeMode="contain"
            style={{ height, aspectRatio, maxWidth: '100%', alignSelf: 'flex-start' }}
        />
    )
}

class HelpRenderer extends Renderer {
    // translateKeys flips per-platform: true on non-Mac, false on
    // Mac-like (iOS / macOS / web in a Mac UA). The text() override
    // below applies the swap only when this flag is set, so docs
    // authored once render natively on every platform.
    private readonly translateKeys: boolean
    private readonly shortcutTables: boolean
    private readonly onLinkPress?: LinkPressHandler
    private readonly transformImageUri?: ImageUriTransform
    private readonly imageMaxHeight?: number

    constructor(options: RendererOptions) {
        super()
        this.translateKeys = options.translateKeys
        this.shortcutTables = options.shortcutTables
        this.onLinkPress = options.onLinkPress
        this.transformImageUri = options.transformImageUri
        this.imageMaxHeight = options.imageMaxHeight
    }

    // The default image() fetches the uri as-is, which 404s for a protected
    // PocketBase file whose stored src is deliberately tokenless. The
    // transform runs here — render time — so every draw carries a live token.
    //
    // The height cap cannot ride the style param: the library's MDImage
    // hardcodes `width: '100%'` + the intrinsic aspect ratio on its OUTER
    // box and applies the passed style only to the inner image — so a
    // maxHeight there letterboxes the pixels while the layout box stays
    // full-bleed. A capped surface therefore renders its own image.
    override image(uri: string, alt?: string, style?: ImageStyle, title?: string): ReactNode {
        const resolved = this.transformImageUri ? this.transformImageUri(uri) : uri
        if (this.imageMaxHeight) {
            return (
                <CappedImage
                    key={this.getKey()}
                    uri={resolved}
                    alt={alt || title}
                    maxHeight={this.imageMaxHeight}
                />
            )
        }
        return super.image(resolved, alt, style, title)
    }

    override link(
        children: string | ReactNode[],
        href: string,
        styles?: TextStyle,
        title?: string
    ): ReactNode {
        return (
            <Text
                key={this.getKey()}
                accessibilityRole="link"
                accessibilityLabel={title || 'Link'}
                onPress={() => handleLinkPress(href, this.onLinkPress)}
                style={styles}
            >
                {children}
            </Text>
        )
    }

    // text() is reached for every prose text run — paragraph text,
    // bold/italic content, table cells, list items. codespan and code
    // are separate tokens and bypass this path, so a literal ⌘ inside
    // an inline `code` span renders verbatim.
    override text(text: string | ReactNode[], styles?: TextStyle): ReactNode {
        if (!this.translateKeys || typeof text !== 'string') {
            return super.text(text, styles)
        }
        return super.text(translateModifierKeys(text), styles)
    }

    // The library's default table() wraps everything in a horizontal
    // ScrollView and gives each cell ~43% of the *window* width, so
    // tables inside a narrow drawer push their second column past the
    // right edge. Render flex rows that fill the container instead.
    override table(
        header: ReactNode[][],
        rows: ReactNode[][][],
        tableStyle?: ViewStyle,
        rowStyle?: ViewStyle,
        cellStyle?: ViewStyle
    ): ReactNode {
        // Each cell paints its right + bottom border; the outer wrapper
        // paints top + left. That gives a single-pixel grid with no
        // doubled-up lines and works without relying on the library's
        // reanimated-table Cell (which we replaced to fix overflow).
        // tableStyle.borderColor always flows in from the themed styles map
        // below (styles.table.borderColor = useThemeColor('border')), so the
        // grid picks up the theme's border color. The `transparent` fallback
        // only guards the never-hit case where the renderer is used without our
        // styles map — it must not hardcode a raw color that breaks in dark mode.
        const borderColor = tableStyle?.borderColor ?? 'transparent'
        const rowFlex: ViewStyle = { flexDirection: 'row', ...rowStyle }
        // Detect keyboard-shortcut tables (first column holds the
        // shortcut, second the description) and give them a 20/80
        // split — equal columns waste space on the keystroke side
        // and crowd the description.
        const isShortcutTable = this.shortcutTables && looksLikeShortcutTable(rows)
        const cellFlexFor = (col: number): ViewStyle => ({
            flex: isShortcutTable ? (col === 0 ? 1 : 4) : 1,
            flexShrink: 1,
            borderRightWidth: 1,
            borderBottomWidth: 1,
            borderColor,
            ...cellStyle,
        })
        const outer: ViewStyle = {
            ...tableStyle,
            borderTopWidth: 1,
            borderLeftWidth: 1,
            borderColor,
        }
        return (
            <View key={this.getKey()} style={outer}>
                <View style={rowFlex}>
                    {header.map((cell, i) => (
                        <View key={`h-${i}`} style={cellFlexFor(i)}>
                            {cell}
                        </View>
                    ))}
                </View>
                {rows.map((row, ri) => (
                    <View key={`r-${ri}`} style={rowFlex}>
                        {row.map((cell, ci) => (
                            <View key={`c-${ri}-${ci}`} style={cellFlexFor(ci)}>
                                {cell}
                            </View>
                        ))}
                    </View>
                ))}
            </View>
        )
    }
}

// looksLikeShortcutTable returns true when the table's first column
// reads as keyboard shortcuts: every row's first cell either contains
// a Mac glyph (⌘ ⇧ ⌥) or one of the cross-platform key tokens that
// translateModifierKeys substitutes in on Windows/Linux/Android
// (Ctrl, Shift, Alt). This isn't a perfect classifier, but it's good
// enough for help topics — and the worst-case failure mode (a normal
// table getting a 20/80 split) is mild visual quirkiness, not a bug.
function looksLikeShortcutTable(rows: ReactNode[][][]): boolean {
    if (rows.length === 0) return false
    const SHORTCUT_PATTERN = /[⌘⇧⌥]|\b(Ctrl|Shift|Alt)\b/
    let hits = 0
    for (const row of rows) {
        if (row.length < 2) return false
        const text = extractCellText(row[0])
        if (SHORTCUT_PATTERN.test(text)) hits++
    }
    // Tolerate one row that doesn't match — for example a separator
    // row or a free-text aside — but require the table to be
    // overwhelmingly shortcut-shaped.
    return hits >= rows.length - 1 && hits >= 1
}

function extractCellText(cell: ReactNode): string {
    if (cell == null || typeof cell === 'boolean') return ''
    if (typeof cell === 'string' || typeof cell === 'number') return String(cell)
    if (Array.isArray(cell)) return cell.map(extractCellText).join('')
    if (typeof cell === 'object' && 'props' in cell) {
        const children = (cell as { props: { children?: ReactNode } }).props.children
        return extractCellText(children)
    }
    return ''
}

// Renderer instances are cached by their option tuple rather than allocated
// per render — react-native-marked holds no per-render state, and a fresh
// renderer each pass would churn the whole token tree. The key space is tiny
// (two booleans plus link-handler identity), so this stays bounded in
// practice: a consumer passes the same handler reference every render.
const rendererCache = new Map<string, HelpRenderer>()

function rendererFor(options: RendererOptions): HelpRenderer {
    // Per-consumer functions can't be stringified, so each gets an identity
    // tag: same function object → same cache slot.
    const key = `${options.translateKeys}|${options.shortcutTables}|${functionTag(options.onLinkPress)}|${functionTag(options.transformImageUri)}|${options.imageMaxHeight ?? 'none'}`
    const cached = rendererCache.get(key)
    if (cached) return cached
    const renderer = new HelpRenderer(options)
    rendererCache.set(key, renderer)
    return renderer
}

const functionTags = new WeakMap<object, number>()
let nextFunctionTag = 1

function functionTag(handler?: object): string {
    if (!handler) return 'none'
    let tag = functionTags.get(handler)
    if (tag === undefined) {
        tag = nextFunctionTag++
        functionTags.set(handler, tag)
    }
    return String(tag)
}

export function MarkdownRenderer({
    body,
    onLinkPress,
    // Aliased on destructure: the prop shares a name with the module-level
    // translateModifierKeys() helper, and an unaliased binding would shadow it
    // for anyone who later reaches for the function in this scope.
    translateModifierKeys: shouldTranslateKeys = true,
    shortcutTableHeuristic = true,
    transformImageUri,
    imageMaxHeight,
}: Props) {
    // Codespan text uses `primary` (the brand teal — has matching
    // light + dark tokens) rather than `accent`. `accent` in this
    // theme is a near-white background fill, so `color: accent`
    // rendered as white-on-white in light mode.
    const [foreground, muted, codeColor, link, surfaceSecondary, border] = [
        useThemeColor('foreground'),
        useThemeColor('muted-foreground'),
        useThemeColor('primary'),
        useThemeColor('link'),
        useThemeColor('surface-secondary'),
        useThemeColor('border'),
    ]

    const styles = useMemo<MarkedStyles>(
        () => ({
            text: { color: foreground, fontSize: 15, lineHeight: 22 },
            paragraph: { marginVertical: 6 },
            em: { fontStyle: 'italic' },
            strong: { fontWeight: '600' },
            link: { color: link, textDecorationLine: 'underline' },
            h1: {
                color: foreground,
                fontSize: 24,
                fontWeight: '700',
                marginTop: 16,
                marginBottom: 8,
            },
            h2: {
                color: foreground,
                fontSize: 20,
                fontWeight: '600',
                marginTop: 16,
                marginBottom: 6,
            },
            h3: {
                color: foreground,
                fontSize: 17,
                fontWeight: '600',
                marginTop: 12,
                marginBottom: 4,
            },
            h4: {
                color: foreground,
                fontSize: 15,
                fontWeight: '600',
                marginTop: 10,
                marginBottom: 4,
            },
            h5: { color: muted, fontSize: 14, fontWeight: '600', marginTop: 8 },
            h6: { color: muted, fontSize: 13, fontWeight: '600', marginTop: 8 },
            codespan: {
                color: codeColor,
                backgroundColor: surfaceSecondary,
                fontFamily: Platform.select({
                    ios: 'Menlo',
                    android: 'monospace',
                    default: 'monospace',
                }),
                fontSize: 13,
                paddingHorizontal: 4,
                borderRadius: 4,
            },
            code: {
                backgroundColor: surfaceSecondary,
                borderRadius: 6,
                padding: 12,
                borderWidth: 1,
                borderColor: border,
            },
            blockquote: {
                borderLeftWidth: 3,
                borderLeftColor: border,
                paddingLeft: 12,
                marginVertical: 8,
            },
            hr: { borderBottomColor: border, borderBottomWidth: 1, marginVertical: 12 },
            list: { marginVertical: 6 },
            li: { color: foreground, fontSize: 15, lineHeight: 22 },
            // The library merges these with its defaults and passes them
            // to HelpRenderer.table(), which then draws the per-cell grid.
            // Setting borderWidth: 0 here suppresses the library's
            // 4-sided outer border so our top/left edges aren't doubled.
            table: { borderColor: border, borderWidth: 0 },
            tableCell: { padding: 8 },
        }),
        [foreground, muted, codeColor, link, surfaceSecondary, border]
    )

    // The glyph swap only ever applies off Mac — on a Mac the source glyphs
    // are already correct, so the flag and the platform are ANDed here rather
    // than in the renderer.
    const renderer = rendererFor({
        translateKeys: shouldTranslateKeys && !isMacLike(),
        shortcutTables: shortcutTableHeuristic,
        onLinkPress,
        transformImageUri,
        imageMaxHeight,
    })

    return (
        <Markdown
            value={body}
            styles={styles}
            renderer={renderer}
            flatListProps={{
                initialNumToRender: 8,
                scrollEnabled: false,
                contentContainerStyle: { paddingBottom: 8 },
                // Override the library's hardcoded #fff/#000 scheme background
                // (its own style loses to flatListProps). An OPAQUE box here
                // paints over anything a caller's negative margin pulls it
                // across — cards' comment rows lost the bottom of their
                // author line to exactly that — and a raw hex never matches
                // the themed surface it sits on anyway.
                style: { backgroundColor: 'transparent' },
            }}
        />
    )
}
