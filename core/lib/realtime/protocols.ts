// The realtime server's credential handshake (core/server/realtime/credentials.go).
// A browser cannot set headers on a WebSocket upgrade, and a credential in
// the URL lands in request and proxy logs, so the credential travels as a
// WebSocket subprotocol. The server answers with REALTIME_PROTOCOL.

export const REALTIME_PROTOCOL = 'tinycld.realtime'
const AUTH_PROTOCOL_PREFIX = 'tinycld.auth.'
const SHARE_PROTOCOL_PREFIX = 'tinycld.share.'

/**
 * The subprotocols for one connection attempt: a share-link visitor's
 * session token when there is one, else the signed-in user's auth token.
 */
export function realtimeProtocols(credentials: {
    authToken: string
    shareSession?: string
}): string[] {
    if (credentials.shareSession) {
        return [REALTIME_PROTOCOL, `${SHARE_PROTOCOL_PREFIX}${credentials.shareSession}`]
    }
    if (credentials.authToken) {
        return [REALTIME_PROTOCOL, `${AUTH_PROTOCOL_PREFIX}${credentials.authToken}`]
    }
    return [REALTIME_PROTOCOL]
}
