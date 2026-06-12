import { PassThrough } from 'node:stream'
import { describe, expect, it } from 'vitest'
import { waitForCurrentId } from '../log-watcher'

function feed(stream: PassThrough, lines: string[]) {
    for (const l of lines) stream.write(`${l}\n`)
}

describe('waitForCurrentId', () => {
    it('resolves with the matching id once a line reports it', async () => {
        const stream = new PassThrough()
        const promise = waitForCurrentId(stream, {
            predicate: id => id === 'build-1718200000000-ios',
            timeoutMs: 1000,
        })
        feed(stream, [
            'msg="app-update: request" q.currentId=embedded-1.13.7',
            'msg="app-update: request" q.currentId=build-1718200000000-ios',
        ])
        await expect(promise).resolves.toBe('build-1718200000000-ios')
    })

    it('reports the last-seen id via onSeen for diagnostics', async () => {
        const stream = new PassThrough()
        const seen: string[] = []
        const promise = waitForCurrentId(stream, {
            predicate: id => id === 'never',
            timeoutMs: 50,
            onSeen: id => seen.push(id),
        })
        feed(stream, ['msg="app-update: request" q.currentId=embedded-1.13.7'])
        await expect(promise).rejects.toThrow(/timed out/i)
        expect(seen).toEqual(['embedded-1.13.7'])
    })

    it('rejects with the last-seen id in the message on timeout', async () => {
        const stream = new PassThrough()
        const promise = waitForCurrentId(stream, {
            predicate: () => false,
            timeoutMs: 50,
        })
        feed(stream, ['msg="app-update: request" q.currentId=embedded-1.13.7'])
        await expect(promise).rejects.toThrow(/embedded-1.13.7/)
    })

    it('rejects promptly when the stream ends before a match', async () => {
        const stream = new PassThrough()
        const promise = waitForCurrentId(stream, {
            predicate: () => false,
            timeoutMs: 10_000,
        })
        feed(stream, ['msg="app-update: request" q.currentId=embedded-1.13.7'])
        stream.end()
        await expect(promise).rejects.toThrow(/stream ended before a match/i)
        await expect(promise).rejects.toThrow(/embedded-1.13.7/)
    })

    it('rejects with the stream error when the stream errors', async () => {
        const stream = new PassThrough()
        const promise = waitForCurrentId(stream, {
            predicate: () => false,
            timeoutMs: 10_000,
        })
        stream.destroy(new Error('boom'))
        await expect(promise).rejects.toThrow(/boom/)
    })
})
