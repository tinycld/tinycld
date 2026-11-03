import type * as http from 'node:http'

/**
 * The headers the dev proxy sends upstream. PocketBase gets the Host the
 * browser sent, as a production reverse proxy passes it: a same-origin check
 * there (a web form's submit compares Origin with Host) must see the page's
 * origin, not the proxy's upstream address. Expo's dev server gets its own
 * host.
 */
export function upstreamHeaders(
    headers: http.IncomingHttpHeaders,
    target: number,
    toPocketBase: boolean
): http.OutgoingHttpHeaders {
    const ownHost = `127.0.0.1:${target}`
    if (toPocketBase) return { ...headers, host: headers.host ?? ownHost }
    return { ...headers, host: ownHost }
}
