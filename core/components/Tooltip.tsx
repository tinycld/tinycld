import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useWebStyles } from '@tinycld/core/lib/use-web-styles'
import type { ReactNode } from 'react'
import { Platform } from 'react-native'

/**
 * The label an icon-only control shows on hover.
 *
 * WEB ONLY, and deliberately so. A tooltip is a hover affordance and touch has
 * no hover; the alternative gestures are all spoken for — a long press is the
 * card drag on boards (lib/dnd.ts's CARD_DRAG_ACTIVATION_MS, tuned so a quick
 * swipe still reads as a scroll), and taking it for a label would cost a
 * shipped interaction to state something the platform already reads aloud. On
 * native this is a passthrough and the control's own `accessibilityLabel` is
 * what TalkBack and VoiceOver announce — so pass BOTH, from the same string.
 *
 * CSS rather than a portaled JS popover: an icon toolbar has a dozen of these
 * and none of them may render, measure, or take focus from the surface the
 * toolbar acts on. `::after` costs nothing until hover.
 *
 * Consolidates three copies that had already drifted — core's own HoverAction,
 * text's ToolbarTooltip and a private one in mail's EmailHeader — which is why
 * it carries the union of what they each learned:
 *
 *  - the 200ms delay (text's), so scanning a toolbar does not strobe;
 *  - `above` / `below` (HoverAction's), because react-native-web leaves
 *    `overflow: hidden` on sibling rows and an above-tooltip on the first row
 *    is clipped by the row above it;
 *  - theme colors through CSS custom properties, so the sheet stays one
 *    constant string both platforms can share rather than a per-render style.
 *
 * NEVER place this BETWEEN a Menu/Popover `trigger` prop and its Pressable. The
 * surface clones its trigger to inject `onPress` and a ref it measures for
 * placement, and this is a Fragment on native — it would swallow both and the
 * menu would never open. Make such a trigger a forwardRef component that wraps
 * its OWN Pressable in a Tooltip.
 */
const tooltipCSS = `
    .tinycld-tooltip {
        position: relative;
        display: inline-flex;
    }
    .tinycld-tooltip::after {
        content: attr(data-tooltip);
        position: absolute;
        left: 50%;
        transform: translateX(-50%);
        padding: 4px 8px;
        border-radius: 4px;
        font-size: 12px;
        line-height: 1;
        white-space: nowrap;
        pointer-events: none;
        opacity: 0;
        transition: opacity 0.15s ease-in 0.2s;
        background: var(--tinycld-tooltip-bg);
        color: var(--tinycld-tooltip-fg);
        z-index: 10;
    }
    .tinycld-tooltip.tinycld-tooltip-above::after {
        bottom: calc(100% + 6px);
    }
    .tinycld-tooltip.tinycld-tooltip-below::after {
        top: calc(100% + 6px);
    }
    .tinycld-tooltip:hover::after {
        opacity: 1;
    }
`

export interface TooltipProps {
    /** What the control does. Pass the same string as its accessibilityLabel. */
    label: string
    /**
     * Which side the bubble opens on. `below` for a control in the top row of
     * a scrolling list, where an above-bubble is clipped by the row above it.
     */
    position?: 'above' | 'below'
    children: ReactNode
}

export function Tooltip({ label, position = 'above', children }: TooltipProps) {
    useWebStyles('tinycld-tooltip', tooltipCSS)
    // Inverted against the page, the way a tooltip reads on every platform:
    // the bubble is foreground-on-background, not another card.
    const background = useThemeColor('foreground')
    const foreground = useThemeColor('background')

    if (Platform.OS !== 'web') return <>{children}</>

    const themeVars = {
        '--tinycld-tooltip-bg': background,
        '--tinycld-tooltip-fg': foreground,
    }

    return (
        <div
            data-tooltip={label}
            className={`tinycld-tooltip tinycld-tooltip-${position}`}
            style={themeVars as never}
        >
            {children}
        </div>
    )
}
