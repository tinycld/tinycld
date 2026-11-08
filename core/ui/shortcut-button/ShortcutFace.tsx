import type { ShortcutHint } from '@tinycld/core/lib/shortcuts/hint'
import { useModifierHeld } from '@tinycld/core/lib/shortcuts/modifier-held'
import { useWebStyles } from '@tinycld/core/lib/use-web-styles'
import { ButtonText } from '@tinycld/core/ui/button'
import { Platform } from 'react-native'

// One grid cell holds both faces, so the button is as wide as the wider of
// the two and never changes size when they swap. `visibility` (not opacity)
// keeps the hidden face's box and takes it out of the accessibility tree.
const faceCSS = `
    .tinycld-shortcut-face {
        display: inline-grid;
        align-items: center;
        justify-items: center;
    }
    .tinycld-shortcut-face > span {
        grid-area: 1 / 1;
    }
    .tinycld-shortcut-face > span[data-shown="false"] {
        visibility: hidden;
    }
`

interface ShortcutFaceProps {
    label: string
    hint: ShortcutHint
}

/**
 * The button's text: its label, or its shortcut while ⌘/Ctrl is held. Web
 * only — native cannot see a modifier held alone, so it shows the label.
 */
export function ShortcutFace({ label, hint }: ShortcutFaceProps) {
    useWebStyles('tinycld-shortcut-face', faceCSS)
    const isHeld = useModifierHeld()

    if (Platform.OS !== 'web') return <ButtonText>{label}</ButtonText>

    const hintText = hint.isPlain ? `then ${hint.text}` : hint.text
    const hintClass = hint.isPlain ? 'opacity-70' : undefined

    return (
        <span className="tinycld-shortcut-face">
            <span data-shown={String(!isHeld)}>
                <ButtonText>{label}</ButtonText>
            </span>
            <span data-shown={String(isHeld)} data-testid="shortcut-hint" aria-hidden>
                <ButtonText className={hintClass} numberOfLines={1}>
                    {hintText}
                </ButtonText>
            </span>
        </span>
    )
}
