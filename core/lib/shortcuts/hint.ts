import { formatKeys, isApplePlatform } from './keys'

const MODIFIERS = new Set(['$mod', 'Control', 'Meta', 'Alt'])

export interface ShortcutHint {
    text: string
    /**
     * No ⌘/Ctrl/Alt in the keys. The hint shows while ⌘ is held, so a plain
     * `c` would read as ⌘C (copy); the button marks it so the user releases
     * ⌘ before pressing the key.
     */
    isPlain: boolean
}

/** The text a ShortcutButton shows for its keys: "⌘↵" on Apple, "Ctrl+↵" elsewhere. */
export function formatHint(keys: string, isApple = isApplePlatform): ShortcutHint {
    const text = formatKeys(keys, isApple)
        .map(parts => parts.join(isApple ? '' : '+'))
        .join(' ')
    const atoms = keys.split(/\s+/).filter(Boolean)
    const isPlain = !atoms.some(atom => atom.split('+').some(part => MODIFIERS.has(part)))
    return { text, isPlain }
}

/**
 * The `aria-keyshortcuts` value for a single combo. ARIA has no form for a
 * sequence ("g i"), so a sequence gets none.
 */
export function ariaKeyShortcuts(keys: string, isApple = isApplePlatform): string | undefined {
    const atoms = keys.split(/\s+/).filter(Boolean)
    if (atoms.length !== 1) return undefined
    return atoms[0]
        .split('+')
        .map(part => (part === '$mod' ? (isApple ? 'Meta' : 'Control') : part))
        .join('+')
}
