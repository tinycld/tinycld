import { beforeEach, describe, expect, it, vi } from 'vitest'
import { __resetCoreConfigForTests, configureCore } from '../core-config'

const captureException = vi.fn()
const captureMessage = vi.fn()
const addBreadcrumb = vi.fn()

vi.mock('../sentry', () => ({
    captureExceptionToSentry: (...args: unknown[]) => captureException(...args),
    captureMessageToSentry: (...args: unknown[]) => captureMessage(...args),
    addBreadcrumbToSentry: (...args: unknown[]) => addBreadcrumb(...args),
}))

describe('log', () => {
    beforeEach(() => {
        __resetCoreConfigForTests()
        captureException.mockClear()
        captureMessage.mockClear()
        addBreadcrumb.mockClear()
    })

    it('breadcrumbs a below-threshold call without raising an event', async () => {
        configureCore({ brandName: 'T', serverShortcuts: {}, logLevel: 'warn' })
        const { log } = await import('../logger')

        log.debug('gizmos.compose', 'draft saved', { draftId: '1' })

        expect(addBreadcrumb).toHaveBeenCalledWith('gizmos.compose', 'debug', 'draft saved', {
            draftId: '1',
        })
        expect(captureMessage).not.toHaveBeenCalled()
        expect(captureException).not.toHaveBeenCalled()
    })

    it('breadcrumbs AND raises an event at the threshold, tagged with the call level', async () => {
        configureCore({ brandName: 'T', serverShortcuts: {}, logLevel: 'warn' })
        const { log } = await import('../logger')

        log.warn('gizmos.imap', 'reconnect attempt', { attempt: 2 })

        expect(addBreadcrumb).toHaveBeenCalledTimes(1)
        expect(addBreadcrumb).toHaveBeenCalledWith('gizmos.imap', 'warn', 'reconnect attempt', {
            attempt: 2,
        })
        expect(captureMessage).toHaveBeenCalledWith('gizmos.imap', 'warn', 'reconnect attempt', {
            attempt: 2,
        })
    })

    it('routes log.error through captureException without also breadcrumbing itself', async () => {
        configureCore({ brandName: 'T', serverShortcuts: {}, logLevel: 'warn' })
        const { log } = await import('../logger')
        const err = new Error('boom')

        log.error('gizmos.send', err, { messageId: 'm1' })

        expect(captureException).toHaveBeenCalledWith('gizmos.send', err, { messageId: 'm1' })
        expect(addBreadcrumb).not.toHaveBeenCalled()
    })

    it('treats an above-threshold info call as an event when level is debug', async () => {
        configureCore({ brandName: 'T', serverShortcuts: {}, logLevel: 'debug' })
        const { log } = await import('../logger')

        log.info('app.boot', 'ready')

        expect(captureMessage).toHaveBeenCalledWith('app.boot', 'info', 'ready', undefined)
    })
})
