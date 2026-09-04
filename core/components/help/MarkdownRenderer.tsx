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
import { type MarkdownPurpose, type MarkdownScale, markdownScale } from './markdown-purpose'

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
    /**
     * Which surface this is rendering into — see markdown-purpose.ts.
     *
     * Defaults to 'documentation': the values every existing caller was
     * already getting, so the help hub is untouched. `description` matches the
     * card editor to the pixel, so tapping a description to edit it does not
     * reflow the prose; `compact` is a comment in a thread.
     */
    purpose?: MarkdownPurpose
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

/**
 * A heading's line height: the SURFACE's own body ratio, applied to the
 * heading's own size.
 *
 * That is what the editor does — it sets no per-heading line-height, so each
 * heading takes the page's unitless ratio — and the ratio differs per surface:
 * a description is 21/14 (1.5) but a comment is 21/15 (1.4). Hard-coding 1.5
 * therefore matched descriptions by luck and made every rendered comment
 * heading 2px taller than the editor's, so a comment with headings re-spaced
 * itself when tapped.
 */
function headingLineHeight(scale: MarkdownScale, size: number): number {
    return Math.round(size * (scale.bodyLineHeight / scale.bodySize))
}

/**
 * Trailing space below the last block of rendered markdown.
 *
 * EXPORTED because a surface that swaps an editor in where this was must
 * reserve the same amount, or everything below it shifts by the difference the
 * moment someone taps. Boards' inline comment edit is the case, and it used to
 * carry a hand-measured constant that silently went stale whenever this moved.
 */
export const MARKDOWN_TRAILING_SPACE = 8

/**
 * Width of a bullet/number marker box, mirroring @jsamr/react-native-li's own
 * `maxNumOfCodepoints * fontSize * 0.6`.
 *
 * A disc marker is two codepoints (the glyph and its trailing space), which is
 * what the library measures for an unordered list. Derived rather than measured
 * at runtime because it feeds a style computed during render.
 */
function markerBoxWidth(fontSize: number): number {
    return fontSize * 1.2
}

interface RendererOptions {
    translateKeys: boolean
    shortcutTables: boolean
    onLinkPress?: LinkPressHandler
    transformImageUri?: ImageUriTransform
    imageMaxHeight?: number
    /** How far a whole list is indented — see HelpRenderer.list. */
    listIndent: number
    /** Body size, used to reserve the marker box inside that indent. */
    bodySize: number
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
    private readonly listIndent: number
    private readonly bodySize: number

    constructor(options: RendererOptions) {
        super()
        this.listIndent = options.listIndent
        this.bodySize = options.bodySize
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
    /**
     * A whole list, indented as one block.
     *
     * The indent goes on a WRAPPER rather than on the item, because none of the
     * per-item style objects is just the row: `list` is the marker box, and `li`
     * reaches the marker's text style as well as the row's content. Putting a
     * margin on either separated the bullet from its words — the marker stayed
     * at the margin while the text moved right — and knocked the marker off its
     * own baseline. Wrapping moves marker and text together, which is what
     * `padding-left` on `ul` does in the editor.
     */
    override list(
        ordered: boolean,
        li: ReactNode[],
        listStyle?: ViewStyle,
        textStyle?: TextStyle,
        startIndex?: number
    ): ReactNode {
        // The marker box is part of the indent, not additional to it.
        //
        // In the editor `ul { padding-left }` reserves the space and the bullet
        // is drawn INSIDE it, so the text starts at the indent. Here the marker
        // is a sibling box laid out before the text, so wrapping at the full
        // indent put the text a marker-width further right than the editor's.
        // Reserving it keeps the TEXT on the same x in both.
        const inset = Math.max(0, this.listIndent - markerBoxWidth(this.bodySize))
        return (
            <View key={this.getKey()} style={{ marginLeft: inset }}>
                {super.list(ordered, li, listStyle, textStyle, startIndex)}
            </View>
        )
    }

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
    // The indent is part of the key: it varies per PURPOSE, and a renderer
    // cached without it would serve a comment the description's indent.
    const key = `${options.translateKeys}|${options.shortcutTables}|${functionTag(options.onLinkPress)}|${functionTag(options.transformImageUri)}|${options.imageMaxHeight ?? 'none'}|${options.listIndent}|${options.bodySize}`
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
    purpose = 'documentation',
}: Props) {
    const scale = markdownScale(purpose)
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
            text: { color: foreground, fontSize: scale.bodySize, lineHeight: scale.bodyLineHeight },
            // A TOP margin only, never marginVertical.
            //
            // CSS collapses adjoining margins and React Native does not, so
            // `marginVertical` puts the gap in twice between any two blocks —
            // which is why a comment rendered with roughly double the rhythm of
            // the editor that replaces it, and read as airy where the editor
            // read as tight. The editor states the same thing once, as
            // `.ProseMirror > * + * { margin-top }`, and the headings here
            // already follow this rule (marginBottom: 0).
            // paddingVertical: 0 is load-bearing. The library defaults a
            // paragraph to `paddingVertical: 8` and our styles MERGE onto that
            // rather than replace it, so every paragraph carried 16px of
            // padding no other block had — the editor has none, so a rendered
            // description read looser than the editor it swaps with even once
            // the margins matched. Same trap as the h1/h2 border below.
            // The gap BETWEEN blocks, stated once as a top margin (RN margins
            // do not collapse). The leading one it also puts on the first block
            // is cancelled by the consumer's wrapper — see MarkdownText.
            //
            // paddingVertical: 0 is load-bearing. The library defaults a
            // paragraph to `paddingVertical: 8` and our styles MERGE onto that
            // rather than replace it, so every paragraph carried 16px of
            // padding no other block had — the editor has none, so a rendered
            // description read looser than the editor it swaps with even once
            // the margins matched. Same trap as the h1/h2 border below.
            paragraph: { marginTop: scale.paragraphSpacing, paddingVertical: 0 },
            em: { fontStyle: 'italic' },
            strong: { fontWeight: '600' },
            link: { color: link, textDecorationLine: 'underline' },
            // The library gives h1 and h2 a bottom border of their own, which
            // our styles merge ON TOP of rather than replace — so a card
            // comment drew a full-width rule under any `#` heading and read as
            // if it had sections. Zeroed explicitly wherever a rule is wrong
            // for the surface; the help hub still gets one.
            h1: {
                color: foreground,
                fontSize: scale.h1.size,
                // Stated, not inherited: the library defaults a heading to a
                // much looser line than the editor's, which made every rendered
                // heading taller than the one it swaps with. The editor sets no
                // per-heading line-height, so a heading there takes the page's
                // ratio — matched here against the heading's OWN size.
                lineHeight: headingLineHeight(scale, scale.h1.size),
                fontWeight: scale.h1.weight,
                marginTop: scale.h1.marginTop,
                marginBottom: scale.h1.marginBottom,
                ...(scale.headingRule ? {} : { borderBottomWidth: 0, paddingBottom: 0 }),
            },
            h2: {
                color: foreground,
                fontSize: scale.h2.size,
                // Stated, not inherited: the library defaults a heading to a
                // much looser line than the editor's, which made every rendered
                // heading taller than the one it swaps with. The editor sets no
                // per-heading line-height, so a heading there takes the page's
                // ratio — matched here against the heading's OWN size.
                lineHeight: headingLineHeight(scale, scale.h2.size),
                fontWeight: scale.h2.weight,
                marginTop: scale.h2.marginTop,
                marginBottom: scale.h2.marginBottom,
                ...(scale.headingRule ? {} : { borderBottomWidth: 0, paddingBottom: 0 }),
            },
            h3: {
                color: foreground,
                fontSize: scale.h3.size,
                // Stated, not inherited: the library defaults a heading to a
                // much looser line than the editor's, which made every rendered
                // heading taller than the one it swaps with. The editor sets no
                // per-heading line-height, so a heading there takes the page's
                // ratio — matched here against the heading's OWN size.
                lineHeight: headingLineHeight(scale, scale.h3.size),
                fontWeight: scale.h3.weight,
                marginTop: scale.h3.marginTop,
                marginBottom: scale.h3.marginBottom,
            },
            h4: {
                color: foreground,
                fontSize: scale.h4.size,
                // Stated, not inherited: the library defaults a heading to a
                // much looser line than the editor's, which made every rendered
                // heading taller than the one it swaps with. The editor sets no
                // per-heading line-height, so a heading there takes the page's
                // ratio — matched here against the heading's OWN size.
                lineHeight: headingLineHeight(scale, scale.h4.size),
                fontWeight: scale.h4.weight,
                marginTop: scale.h4.marginTop,
                marginBottom: scale.h4.marginBottom,
            },
            // h5/h6 keep the muted color ONLY where headings carry document
            // hierarchy. On a comment they are just small emphasis, and greying
            // them made the deepest heading read as less important than the
            // body text around it.
            h5: {
                color: purpose === 'documentation' ? muted : foreground,
                fontSize: scale.h5.size,
                // Stated, not inherited: the library defaults a heading to a
                // much looser line than the editor's, which made every rendered
                // heading taller than the one it swaps with. The editor sets no
                // per-heading line-height, so a heading there takes the page's
                // ratio — matched here against the heading's OWN size.
                lineHeight: headingLineHeight(scale, scale.h5.size),
                fontWeight: scale.h5.weight,
                marginTop: scale.h5.marginTop,
                marginBottom: scale.h5.marginBottom,
            },
            h6: {
                color: purpose === 'documentation' ? muted : foreground,
                fontSize: scale.h6.size,
                // Stated, not inherited: the library defaults a heading to a
                // much looser line than the editor's, which made every rendered
                // heading taller than the one it swaps with. The editor sets no
                // per-heading line-height, so a heading there takes the page's
                // ratio — matched here against the heading's OWN size.
                lineHeight: headingLineHeight(scale, scale.h6.size),
                fontWeight: scale.h6.weight,
                marginTop: scale.h6.marginTop,
                marginBottom: scale.h6.marginBottom,
            },
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
            // NOT a list box. react-native-marked hands this straight to
            // @jsamr/react-native-li as `markerBoxStyle`, which applies it to
            // each BULLET GLYPH — so a margin here shifts the markers away from
            // their own text rather than indenting the list. Everything the list
            // needs therefore lives on `li` below, which IS the item row.
            // Neither of these may carry GEOMETRY, and that is not obvious from
            // the names. react-native-marked hands `list` to the marker box and
            // `li` to three places at once — the item's text, the item's row,
            // AND the marker's own text style — so a margin on either lands on
            // the bullet as well as the row. That is what indented the text a
            // second time (bullet at one x, words 38px further right) and
            // dropped every marker 8px below its own line.
            //
            // Typography only here; the row's indent and rhythm are applied in
            // HelpRenderer.listItem, which is the one place that IS just the row.
            list: {},
            li: {
                color: foreground,
                fontSize: scale.bodySize,
                lineHeight: scale.bodyLineHeight,
            },
            // The library merges these with its defaults and passes them
            // to HelpRenderer.table(), which then draws the per-cell grid.
            // Setting borderWidth: 0 here suppresses the library's
            // 4-sided outer border so our top/left edges aren't doubled.
            table: { borderColor: border, borderWidth: 0 },
            tableCell: { padding: 8 },
        }),
        [foreground, muted, codeColor, link, surfaceSecondary, border, scale, purpose]
    )

    // The glyph swap only ever applies off Mac — on a Mac the source glyphs
    // are already correct, so the flag and the platform are ANDed here rather
    // than in the renderer.
    const renderer = rendererFor({
        translateKeys: shouldTranslateKeys && !isMacLike(),
        shortcutTables: shortcutTableHeuristic,
        listIndent: scale.listIndent,
        bodySize: scale.bodySize,
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
                contentContainerStyle: { paddingBottom: MARKDOWN_TRAILING_SPACE },
                // Override the library's hardcoded #fff/#000 scheme background
                // (its own style loses to flatListProps). An OPAQUE box here
                // paints over anything a caller's negative margin pulls it
                // across — boards' comment rows lost the bottom of their
                // author line to exactly that — and a raw hex never matches
                // the themed surface it sits on anyway.
                style: { backgroundColor: 'transparent' },
            }}
        />
    )
}
