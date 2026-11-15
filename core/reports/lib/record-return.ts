/**
 * A record a report row opens lands in its package's tab, on top of whatever that
 * tab's stack already holds (spec D2). These helpers read the tabs' navigation state
 * to find the route the open added and to tell when the reader left it by going back.
 * They are pure, so the rule is testable without a router.
 */

/** The part of a navigator's state the helpers read (a partial state has no keys). */
export interface NavNode {
    key?: string
    index?: number
    routes: readonly NavRoute[]
}

export interface NavRoute {
    key?: string
    name: string
    state?: NavNode
}

/** The route an open added, and the routes under it that a Back pops to. */
export interface OpenedRecord {
    tabName: string
    routeKey: string
    navigatorKey: string
    below: readonly string[]
}

export type ReturnStep = 'wait' | 'return' | 'done'

/** The route of the tab named `tabName`. */
export function tabRouteOf(tabs: NavNode, tabName: string): NavRoute | undefined {
    return tabs.routes.find(route => route.name === tabName)
}

/** Every route key in a navigator's state and the states nested in it. */
export function routeKeysOf(state: NavNode | undefined): Set<string> {
    const keys = new Set<string>()
    const visit = (node: NavNode | undefined) => {
        for (const route of node?.routes ?? []) {
            if (route.key) keys.add(route.key)
            visit(route.state)
        }
    }
    visit(state)
    return keys
}

function focusedOf(node: NavNode): NavRoute | undefined {
    return node.routes[node.index ?? node.routes.length - 1]
}

/**
 * The first route on the tab's focused branch that was not there before the open
 * (`before`): the record, or the nested navigator route the push added for it.
 */
export function openedRecordOf(
    tabs: NavNode,
    tabName: string,
    before: ReadonlySet<string>
): OpenedRecord | undefined {
    let node = tabRouteOf(tabs, tabName)?.state
    while (node) {
        const focused = focusedOf(node)
        if (!focused?.key) return undefined
        if (!before.has(focused.key)) {
            if (!node.key) return undefined
            const at = node.routes.indexOf(focused)
            const below = node.routes.slice(0, at).flatMap(route => (route.key ? [route.key] : []))
            return { tabName, routeKey: focused.key, navigatorKey: node.key, below }
        }
        node = focused.state
    }
    return undefined
}

function navigatorOf(node: NavNode | undefined, key: string): NavNode | undefined {
    if (!node) return undefined
    if (node.key === key) return node
    for (const route of node.routes) {
        const found = navigatorOf(route.state, key)
        if (found) return found
    }
    return undefined
}

/**
 * What to do after a change of the tabs' state, while the opened record is watched:
 * `wait` while the record is still in its tab; `return` when the reader went back
 * from it in that tab (the tab is focused and its navigator shows a route that was
 * under the record), so the report is shown again; `done` when it left another way
 * (browser Back restored the report, a replace, the tab was reset).
 */
export function returnStepOf(tabs: NavNode, opened: OpenedRecord): ReturnStep {
    const tab = tabRouteOf(tabs, opened.tabName)
    if (!tab) return 'done'
    if (routeKeysOf(tab.state).has(opened.routeKey)) return 'wait'
    if (focusedOf(tabs)?.name !== opened.tabName) return 'done'
    const navigator = navigatorOf(tab.state, opened.navigatorKey)
    const shown = navigator ? focusedOf(navigator)?.key : undefined
    return shown !== undefined && opened.below.includes(shown) ? 'return' : 'done'
}
