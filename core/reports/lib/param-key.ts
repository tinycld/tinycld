// A report param's key in the route's search params. This module imports
// nothing, so Node code (a package's e2e helpers, CLI scripts) can build a
// report link without loading the app; view-url re-exports it.

export const PARAM_KEY_PREFIX = 'p-'

export function paramKey(id: string): string {
    return `${PARAM_KEY_PREFIX}${id}`
}
