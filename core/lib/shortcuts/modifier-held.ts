import { create } from '@tinycld/core/lib/store'
import { isApplePlatform } from './keys'

/**
 * How long ⌘/Ctrl must be held ALONE before buttons show their shortcuts.
 * ⌘ is pressed constantly for ⌘C, ⌘V and ⌘Tab; with no delay every one of
 * those would flash every label on screen.
 */
export const MODIFIER_HOLD_MS = 400

export interface ModifierKeyEvent {
    key: string
    repeat: boolean
    shiftKey: boolean
    altKey: boolean
    ctrlKey: boolean
    metaKey: boolean
}

interface TrackerOptions {
    modifier: 'Meta' | 'Control'
    holdMs?: number
    onChange: (isHeld: boolean) => void
}

/**
 * The hold rules as a plain state machine, so a unit test can drive them
 * with fake timers. `watchModifierHeld` only feeds it DOM events.
 */
export function createModifierTracker({
    modifier,
    holdMs = MODIFIER_HOLD_MS,
    onChange,
}: TrackerOptions) {
    let timer: ReturnType<typeof setTimeout> | null = null
    let isHeld = false

    const set = (next: boolean) => {
        if (isHeld === next) return
        isHeld = next
        onChange(next)
    }

    const reset = () => {
        if (timer) clearTimeout(timer)
        timer = null
        set(false)
    }

    const otherModifierDown = (e: ModifierKeyEvent) =>
        e.shiftKey || e.altKey || (modifier === 'Meta' ? e.ctrlKey : e.metaKey)

    return {
        keyDown(e: ModifierKeyEvent) {
            if (e.key !== modifier) {
                reset()
                return
            }
            if (e.repeat || timer || isHeld) return
            if (otherModifierDown(e)) return
            timer = setTimeout(() => {
                timer = null
                set(true)
            }, holdMs)
        },
        keyUp(e: ModifierKeyEvent) {
            if (e.key === modifier) reset()
        },
        reset,
    }
}

interface ModifierHeldState {
    isHeld: boolean
}

export const useModifierHeldStore = create<ModifierHeldState>(() => ({ isHeld: false }))

/** True while ⌘ (Apple) or Ctrl is held alone. Always false on native. */
export function useModifierHeld(): boolean {
    return useModifierHeldStore(s => s.isHeld)
}

/**
 * Web only. Capture phase, because react-native-web's TextInput stops every
 * keydown before a bubble listener would see it, and the hint must also show
 * while a field has focus. `blur` and a hidden page reset the hold: ⌘Tab
 * moves focus away before the keyup, which this window then never receives.
 */
export function watchModifierHeld(win: Window, doc: Document): () => void {
    const tracker = createModifierTracker({
        modifier: isApplePlatform ? 'Meta' : 'Control',
        onChange: isHeld => useModifierHeldStore.setState({ isHeld }),
    })
    const onKeyDown = (e: KeyboardEvent) => tracker.keyDown(e)
    const onKeyUp = (e: KeyboardEvent) => tracker.keyUp(e)
    const onVisibility = () => {
        if (doc.visibilityState === 'hidden') tracker.reset()
    }

    win.addEventListener('keydown', onKeyDown, true)
    win.addEventListener('keyup', onKeyUp, true)
    win.addEventListener('blur', tracker.reset)
    doc.addEventListener('visibilitychange', onVisibility)
    return () => {
        tracker.reset()
        win.removeEventListener('keydown', onKeyDown, true)
        win.removeEventListener('keyup', onKeyUp, true)
        win.removeEventListener('blur', tracker.reset)
        doc.removeEventListener('visibilitychange', onVisibility)
    }
}
