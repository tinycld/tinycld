import { type Href, Link } from 'expo-router'
import type { ReactNode } from 'react'
import {
    Platform,
    Pressable,
    type PressableStateCallbackType,
    Text,
    View,
    type ViewStyle,
} from 'react-native'
import type { RowLinks } from '../hooks/useRowLinks'
import { useFigureRef } from '../lib/figure-focus'
import { linkPress } from '../lib/link-press'
import type { ResultRow } from '../lib/types'

// react-native-web adds `hovered` to a Pressable's state; native has no pointer.
function isHovered(state: PressableStateCallbackType): boolean {
    return 'hovered' in state && state.hovered === true
}

/** A link's text: underlined under the pointer on web, dimmed while pressed on native. */
export function linkTone(state: PressableStateCallbackType): string {
    if (isHovered(state)) return 'underline'
    return Platform.OS !== 'web' && state.pressed ? 'opacity-60' : ''
}

interface DrillFigureProps {
    href: Href
    testID: string
    style: { width: number | undefined; minHeight: number; flexGrow: number }
    /** "Show transactions: <row label>, <column heading>, <figure>". */
    name: string
    text: string
    textClass: string
    lines: number
    /** The drill panel shows exactly this figure's params. */
    isShown: boolean
    /** Part of a stuck copy: no tab stop, hidden from assistive technology. */
    isCopy: boolean
    /** The figure's key (figureKey), under which it takes the focus back when the panel closes. */
    figureKey: string
    onPress(): void
}

/**
 * A figure that opens the rows behind it. Its href is always the full report, so
 * ⌘-click and middle-click open it in a new tab; a plain press runs the view's
 * drill (a panel, or the full report as a screen).
 */
export function DrillFigure(props: DrillFigureProps) {
    const marked = props.isShown ? 'bg-accent' : ''
    const ref = useFigureRef(props.figureKey, props.isCopy)
    return (
        <Link href={props.href} asChild>
            <Pressable
                ref={ref}
                testID={props.testID}
                accessibilityRole="link"
                accessibilityLabel={props.name}
                aria-selected={props.isShown}
                accessible={!props.isCopy}
                focusable={!props.isCopy}
                tabIndex={props.isCopy ? -1 : 0}
                onPress={linkPress(props.onPress)}
                className={`justify-center px-3 web:cursor-pointer ${marked}`}
                style={props.style}
            >
                {state => (
                    <Text
                        numberOfLines={props.lines}
                        className={`${props.textClass} ${linkTone(state)}`}
                    >
                        {props.text}
                    </Text>
                )}
            </Pressable>
        </Link>
    )
}

interface OpenRowLabelProps {
    href: Href
    testID: string
    text: string
    textClass: string
    lines: number
    style: ViewStyle
    /** The record panel shows this row's record (R11); the one row marked. */
    isShown: boolean
    /** The label's key (rowFocusKey), under which it takes the focus back when the panel closes. */
    focusKey: string
    onPress(): void
}

/** A row's label that opens its record (D2), named "Open: <label>". */
export function OpenRowLabel(props: OpenRowLabelProps) {
    // The label is the row's one accessible control (RowFrame is not), so it carries
    // the selected state; React Native maps aria-selected to accessibilityState. Only
    // the opened row carries it: ARIA has no selected state for a link, so the other
    // rows leave it out rather than say "false".
    const ref = useFigureRef(props.focusKey, false)
    return (
        <Link href={props.href} asChild>
            <Pressable
                ref={ref}
                testID={props.testID}
                accessibilityRole="link"
                accessibilityLabel={`Open: ${props.text}`}
                aria-selected={props.isShown || undefined}
                onPress={linkPress(props.onPress)}
                className="flex-row items-center pr-3 web:cursor-pointer"
                style={props.style}
            >
                {state => (
                    <Text
                        numberOfLines={props.lines}
                        className={`${props.textClass} ${linkTone(state)}`}
                    >
                        {props.text}
                    </Text>
                )}
            </Pressable>
        </Link>
    )
}

interface RowFrameProps {
    row: ResultRow
    links: RowLinks | undefined
    isCopy: boolean | undefined
    className: string
    children: ReactNode
}

/**
 * A row's box. A row that opens its record takes a press anywhere on it; its label
 * link is the one control in the tab order and for assistive technology.
 */
export function RowFrame({ row, links, isCopy, className, children }: RowFrameProps) {
    const href = links && !isCopy ? links.rowHref(row) : null
    // The row whose record the panel shows takes the selected-row token, as a shown figure does.
    const tone = links && href !== null && links.isRowShown(row) ? 'bg-accent' : 'bg-background'
    if (!links || href === null) return <View className={`${className} ${tone}`}>{children}</View>
    return (
        <Pressable
            accessible={false}
            focusable={false}
            tabIndex={-1}
            onPress={linkPress(() => links.onRow(row))}
            className={`${className} ${tone} web:cursor-pointer`}
        >
            {children}
        </Pressable>
    )
}
