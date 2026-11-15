import { describe, expect, it } from 'vitest'
import { type NavNode, openedRecordOf, returnStepOf, routeKeysOf } from '../record-return'

// A fictional package "zoo" whose stack holds its index and a form the reader left
// half filled, beside the reports tab.
function tabs(focused: 'reports' | 'zoo', zooRoutes: string[]): NavNode {
    return {
        key: 'tabs',
        index: focused === 'reports' ? 0 : 1,
        routes: [
            { key: 'reports-1', name: 'reports', state: { key: 'r', index: 0, routes: [] } },
            {
                key: 'zoo-1',
                name: 'zoo',
                state: {
                    key: 'zoo-stack',
                    index: zooRoutes.length - 1,
                    routes: zooRoutes.map(key => ({ key, name: key.split('-')[0] ?? key })),
                },
            },
        ],
    }
}

const BEFORE = tabs('reports', ['index-1', 'form-1'])
const OPENED = tabs('zoo', ['index-1', 'form-1', 'feeding-1'])

describe('the record a report row opened', () => {
    it('is the route the push added on the tab’s focused branch', () => {
        const opened = openedRecordOf(OPENED, 'zoo', routeKeysOf(BEFORE.routes[1]?.state))
        expect(opened).toEqual({
            tabName: 'zoo',
            routeKey: 'feeding-1',
            navigatorKey: 'zoo-stack',
            below: ['index-1', 'form-1'],
        })
    })

    it('is not found while the push has not landed', () => {
        expect(openedRecordOf(BEFORE, 'zoo', routeKeysOf(BEFORE.routes[1]?.state))).toBeUndefined()
    })

    it('is the nested navigator route when the push added one', () => {
        const nested: NavNode = {
            key: 'tabs',
            index: 1,
            routes: [
                { key: 'reports-1', name: 'reports' },
                {
                    key: 'zoo-1',
                    name: 'zoo',
                    state: {
                        key: 'zoo-stack',
                        index: 1,
                        routes: [
                            { key: 'index-1', name: 'index' },
                            {
                                key: 'feedings-1',
                                name: 'feedings',
                                state: {
                                    key: 'f',
                                    index: 0,
                                    routes: [{ key: 'id-1', name: '[id]' }],
                                },
                            },
                        ],
                    },
                },
            ],
        }
        expect(openedRecordOf(nested, 'zoo', new Set(['index-1']))?.routeKey).toBe('feedings-1')
    })
})

describe('the step after a change of the tabs', () => {
    const opened = openedRecordOf(OPENED, 'zoo', routeKeysOf(BEFORE.routes[1]?.state))
    if (!opened) throw new Error('no opened record')

    it('waits while the record is in its tab, focused or not', () => {
        expect(returnStepOf(OPENED, opened)).toBe('wait')
        expect(returnStepOf(tabs('reports', ['index-1', 'form-1', 'feeding-1']), opened)).toBe(
            'wait'
        )
    })

    it('returns to the report after a Back from the record in its tab', () => {
        expect(returnStepOf(tabs('zoo', ['index-1', 'form-1']), opened)).toBe('return')
    })

    it('is done when the record left with the report shown (browser Back)', () => {
        expect(returnStepOf(tabs('reports', ['index-1', 'form-1']), opened)).toBe('done')
    })

    it('is done when the record was replaced', () => {
        expect(returnStepOf(tabs('zoo', ['index-1', 'form-1', 'other-1']), opened)).toBe('done')
    })
})
