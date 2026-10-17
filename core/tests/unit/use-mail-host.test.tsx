// @vitest-environment happy-dom
import { renderHook } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { setResolvedAddress } from '../../lib/server-address'

const h = vi.hoisted(() => ({ info: { mailHost: '', isPending: false } }))

vi.mock('../../lib/use-org-info', () => ({ useOrgInfo: () => h.info }))

import { useMailHost } from '../../lib/use-mail-host'

afterEach(() => setResolvedAddress(null))

test('is null while org-info is pending', () => {
    setResolvedAddress('https://acme.tinycld.org')
    h.info = { mailHost: '', isPending: true }
    expect(renderHook(() => useMailHost()).result.current).toBeNull()
})

test('is the configured mail host', () => {
    setResolvedAddress('https://acme.tinycld.org')
    h.info = { mailHost: 'mx.acme.example', isPending: false }
    expect(renderHook(() => useMailHost()).result.current).toBe('mx.acme.example')
})

test('falls back to the host the app talks to', () => {
    setResolvedAddress('http://localhost:7100')
    h.info = { mailHost: '', isPending: false }
    expect(renderHook(() => useMailHost()).result.current).toBe('localhost')
})
