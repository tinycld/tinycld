import {
    createAwayClock,
    createCatchUp,
    createDebouncedTrigger,
    isBackground,
    isDisconnect,
    isInTouch,
    isReconnect,
    isResume,
    RELOAD_AFTER_AWAY_MS,
} from '@tinycld/core/lib/reload-on-resume'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

describe('isResume', () => {
    it('is a move into the foreground', () => {
        expect(isResume('background', 'active')).toBe(true)
        expect(isResume('inactive', 'active')).toBe(true)
        expect(isResume('active', 'active')).toBe(false)
        expect(isResume('active', 'background')).toBe(false)
    })
})

describe('isReconnect', () => {
    const up = { isOnline: true, isServerReachable: true }
    it('is the network or the server coming back', () => {
        expect(isReconnect({ isOnline: false, isServerReachable: true }, up)).toBe(true)
        expect(isReconnect({ isOnline: true, isServerReachable: false }, up)).toBe(true)
        expect(isReconnect(up, up)).toBe(false)
        expect(isReconnect(up, { isOnline: false, isServerReachable: true })).toBe(false)
    })
})

describe('createDebouncedTrigger', () => {
    beforeEach(() => {
        vi.useFakeTimers()
    })
    afterEach(() => {
        vi.useRealTimers()
    })

    it('runs once, after the last of a burst of triggers', async () => {
        const run = vi.fn(async () => {})
        const reload = createDebouncedTrigger(run, 1000, vi.fn())

        reload.trigger()
        await vi.advanceTimersByTimeAsync(600)
        reload.trigger()
        await vi.advanceTimersByTimeAsync(600)
        expect(run).not.toHaveBeenCalled()

        await vi.advanceTimersByTimeAsync(400)
        expect(run).toHaveBeenCalledOnce()
    })

    it('does not run once cancelled', async () => {
        const run = vi.fn(async () => {})
        const reload = createDebouncedTrigger(run, 1000, vi.fn())
        reload.trigger()
        reload.cancel()
        await vi.advanceTimersByTimeAsync(2000)
        expect(run).not.toHaveBeenCalled()
    })

    it('reports a failed run instead of rejecting unhandled', async () => {
        const error = new Error('offline')
        const onError = vi.fn()
        const reload = createDebouncedTrigger(() => Promise.reject(error), 10, onError)
        reload.trigger()
        await vi.advanceTimersByTimeAsync(10)
        expect(onError).toHaveBeenCalledWith(error)
    })
})

describe('isBackground', () => {
    it('is a move out of the foreground', () => {
        expect(isBackground('active', 'background')).toBe(true)
        expect(isBackground('active', 'inactive')).toBe(true)
        expect(isBackground('background', 'inactive')).toBe(false)
        expect(isBackground('background', 'active')).toBe(false)
    })
})

describe('isDisconnect', () => {
    const up = { isOnline: true, isServerReachable: true }
    it('is the network or the server going away', () => {
        expect(isDisconnect(up, { isOnline: false, isServerReachable: true })).toBe(true)
        expect(isDisconnect(up, { isOnline: true, isServerReachable: false })).toBe(true)
        expect(isDisconnect(up, up)).toBe(false)
        expect(isDisconnect({ isOnline: false, isServerReachable: true }, up)).toBe(false)
    })
})

describe('isInTouch', () => {
    const up = { isOnline: true, isServerReachable: true }
    it('needs the foreground, the network and the server', () => {
        expect(isInTouch('active', up)).toBe(true)
        expect(isInTouch('background', up)).toBe(false)
        expect(isInTouch('active', { isOnline: false, isServerReachable: true })).toBe(false)
        expect(isInTouch('active', { isOnline: true, isServerReachable: false })).toBe(false)
    })
})

describe('createAwayClock', () => {
    it('measures from the first time the app left', () => {
        let now = 1000
        const away = createAwayClock(() => now)
        away.leave()
        now = 5000
        away.leave()
        now = 9000
        expect(away.back()).toBe(8000)
    })

    it('starts again from zero after a return', () => {
        let now = 0
        const away = createAwayClock(() => now)
        away.leave()
        now = 100
        away.back()
        now = 500
        expect(away.back()).toBe(0)
    })
})

describe('createCatchUp', () => {
    function setup(inTouch: { value: boolean }, reloadAfterLongAbsence = true) {
        let now = 0
        const away = createAwayClock(() => now)
        const calls: string[] = []
        const catchUp = createCatchUp({
            inTouch: () => inTouch.value,
            away,
            reloadAfterLongAbsence,
            reload: async () => {
                calls.push('reload')
            },
            reconnect: () => calls.push('reconnect'),
        })
        return {
            calls,
            catchUp,
            leaveFor(ms: number) {
                away.leave()
                now += ms
            },
        }
    }

    it('only reconnects realtime after a short absence', async () => {
        const app = setup({ value: true })
        app.leaveFor(RELOAD_AFTER_AWAY_MS - 1)
        await app.catchUp()
        expect(app.calls).toEqual(['reconnect'])
    })

    it('reloads after a long absence', async () => {
        const app = setup({ value: true })
        app.leaveFor(RELOAD_AFTER_AWAY_MS)
        await app.catchUp()
        expect(app.calls).toEqual(['reload'])
    })

    it('only reconnects after a long absence when reloading is off (web)', async () => {
        const app = setup({ value: true }, false)
        app.leaveFor(RELOAD_AFTER_AWAY_MS * 2)
        await app.catchUp()
        expect(app.calls).toEqual(['reconnect'])
    })

    it('waits until the app is in touch, and then counts the whole absence', async () => {
        const inTouch = { value: false }
        const app = setup(inTouch)
        app.leaveFor(RELOAD_AFTER_AWAY_MS / 2)
        await app.catchUp()
        expect(app.calls).toEqual([])

        app.leaveFor(RELOAD_AFTER_AWAY_MS / 2)
        inTouch.value = true
        await app.catchUp()
        expect(app.calls).toEqual(['reload'])
    })
})
