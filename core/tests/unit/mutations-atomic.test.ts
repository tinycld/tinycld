import type { Transaction } from '@tanstack/react-db'
import { batch } from 'pbtsdb'
import { describe, expect, it } from 'vitest'
import { atomic, performMutations } from '../../lib/mutations'

function fakeTransaction(settled: Promise<unknown>) {
    return { when: () => settled } as unknown as Transaction<Record<string, unknown>>
}

describe('atomic', () => {
    it('is the pbtsdb batch function', () => {
        expect(atomic).toBe(batch)
    })
})

describe('performMutations with a yielded promise', () => {
    it('waits for the promise before it resumes the generator', async () => {
        const order: string[] = []
        let release = () => {}
        const gate = new Promise<void>(resolve => {
            release = resolve
        })
        const run = performMutations(function* () {
            yield gate.then(() => {
                order.push('batch settled')
            })
            order.push('after the yield')
        })

        await Promise.resolve()
        expect(order).toEqual([])
        release()
        await run
        expect(order).toEqual(['batch settled', 'after the yield'])
    })

    it('rejects with the error of a rejected promise and runs no later step', async () => {
        const failure = new Error('batch failed')
        let ranLaterStep = false
        const run = performMutations(function* () {
            yield Promise.reject(failure)
            ranLaterStep = true
        })
        await expect(run).rejects.toBe(failure)
        expect(ranLaterStep).toBe(false)
    })

    it('waits for a promise and a transaction yielded in one array', async () => {
        const order: string[] = []
        await performMutations(function* () {
            yield [
                Promise.resolve().then(() => {
                    order.push('promise')
                }),
                fakeTransaction(
                    Promise.resolve().then(() => {
                        order.push('transaction')
                    })
                ),
            ]
            order.push('after the yield')
        })
        expect(order.at(-1)).toBe('after the yield')
        expect(order).toHaveLength(3)
    })
})
