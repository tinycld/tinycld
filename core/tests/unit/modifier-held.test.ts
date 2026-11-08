import {
    createModifierTracker,
    MODIFIER_HOLD_MS,
    type ModifierKeyEvent,
} from '@tinycld/core/lib/shortcuts/modifier-held'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

function key(name: string, extra: Partial<ModifierKeyEvent> = {}): ModifierKeyEvent {
    return {
        key: name,
        repeat: false,
        shiftKey: false,
        altKey: false,
        ctrlKey: name === 'Control',
        metaKey: name === 'Meta',
        ...extra,
    }
}

describe('createModifierTracker', () => {
    let onChange: ReturnType<typeof vi.fn>

    beforeEach(() => {
        vi.useFakeTimers()
        onChange = vi.fn()
    })

    afterEach(() => {
        vi.useRealTimers()
    })

    it('reports held after the modifier is held alone for the hold time', () => {
        const t = createModifierTracker({ modifier: 'Meta', onChange })
        t.keyDown(key('Meta'))
        vi.advanceTimersByTime(MODIFIER_HOLD_MS - 1)
        expect(onChange).not.toHaveBeenCalled()
        vi.advanceTimersByTime(1)
        expect(onChange).toHaveBeenLastCalledWith(true)
    })

    it('does not report held when released before the hold time', () => {
        const t = createModifierTracker({ modifier: 'Meta', onChange })
        t.keyDown(key('Meta'))
        vi.advanceTimersByTime(MODIFIER_HOLD_MS / 2)
        t.keyUp(key('Meta', { metaKey: false }))
        vi.advanceTimersByTime(MODIFIER_HOLD_MS)
        expect(onChange).not.toHaveBeenCalledWith(true)
    })

    it('cancels the hold when another key is pressed (⌘C)', () => {
        const t = createModifierTracker({ modifier: 'Meta', onChange })
        t.keyDown(key('Meta'))
        t.keyDown(key('c', { metaKey: true }))
        vi.advanceTimersByTime(MODIFIER_HOLD_MS * 2)
        expect(onChange).not.toHaveBeenCalledWith(true)
    })

    it('clears a shown hint when another key is pressed', () => {
        const t = createModifierTracker({ modifier: 'Meta', onChange })
        t.keyDown(key('Meta'))
        vi.advanceTimersByTime(MODIFIER_HOLD_MS)
        t.keyDown(key('Enter', { metaKey: true }))
        expect(onChange).toHaveBeenLastCalledWith(false)
    })

    it('ignores key repeat of the held modifier', () => {
        const t = createModifierTracker({ modifier: 'Meta', onChange })
        t.keyDown(key('Meta'))
        vi.advanceTimersByTime(MODIFIER_HOLD_MS - 100)
        t.keyDown(key('Meta', { repeat: true }))
        vi.advanceTimersByTime(100)
        expect(onChange).toHaveBeenLastCalledWith(true)
    })

    it('does not start while another modifier is already down (⇧ then ⌘)', () => {
        const t = createModifierTracker({ modifier: 'Meta', onChange })
        t.keyDown(key('Meta', { shiftKey: true }))
        vi.advanceTimersByTime(MODIFIER_HOLD_MS * 2)
        expect(onChange).not.toHaveBeenCalledWith(true)
    })

    it('clears on reset — the window lost focus during ⌘Tab', () => {
        const t = createModifierTracker({ modifier: 'Meta', onChange })
        t.keyDown(key('Meta'))
        vi.advanceTimersByTime(MODIFIER_HOLD_MS)
        t.reset()
        expect(onChange).toHaveBeenLastCalledWith(false)
    })

    it('uses Control as the modifier off Apple, and ignores Meta there', () => {
        const t = createModifierTracker({ modifier: 'Control', onChange })
        t.keyDown(key('Meta'))
        vi.advanceTimersByTime(MODIFIER_HOLD_MS)
        expect(onChange).not.toHaveBeenCalledWith(true)
        t.keyUp(key('Meta', { metaKey: false }))
        t.keyDown(key('Control'))
        vi.advanceTimersByTime(MODIFIER_HOLD_MS)
        expect(onChange).toHaveBeenLastCalledWith(true)
    })

    it('reports each change once', () => {
        const t = createModifierTracker({ modifier: 'Meta', onChange })
        t.reset()
        t.keyUp(key('Meta', { metaKey: false }))
        expect(onChange).not.toHaveBeenCalled()
    })
})
