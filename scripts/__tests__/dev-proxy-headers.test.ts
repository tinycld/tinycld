import { describe, expect, it } from 'vitest'
import { upstreamHeaders } from '../dev-proxy-headers'

describe('upstreamHeaders', () => {
    // A web form's submit compares the page's Origin with the request's Host,
    // so PocketBase must see the Host the browser sent, as behind a real proxy.
    it('keeps the browser Host for PocketBase', () => {
        const headers = upstreamHeaders(
            { host: 'localhost:7120', origin: 'http://localhost:7120' },
            7121,
            true
        )
        expect(headers.host).toBe('localhost:7120')
        expect(headers.origin).toBe('http://localhost:7120')
    })

    it('gives Expo its own host', () => {
        expect(upstreamHeaders({ host: 'localhost:7120' }, 7122, false).host).toBe('127.0.0.1:7122')
    })

    it('fills in a Host for PocketBase when the browser sent none', () => {
        expect(upstreamHeaders({}, 7121, true).host).toBe('127.0.0.1:7121')
    })
})
