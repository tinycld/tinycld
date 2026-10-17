import { afterEach, expect, test, vi } from 'vitest'
import { setResolvedAddress } from '../../lib/server-address'
import { fetchOrgInfo } from '../../lib/use-org-info'

afterEach(() => {
    vi.unstubAllGlobals()
    setResolvedAddress(null)
})

function serve(body: unknown) {
    vi.stubGlobal(
        'fetch',
        vi.fn(async () => new Response(JSON.stringify(body), { status: 200 }))
    )
}

test('reads the mail host from org-info', async () => {
    setResolvedAddress('https://acme.tinycld.org')
    serve({ name: 'Acme', mailHost: 'mx.acme.example' })
    expect((await fetchOrgInfo()).mailHost).toBe('mx.acme.example')
})

// An older server sends no mailHost: that reads as "use the web host".
test('reads a missing or non-string mail host as empty', async () => {
    setResolvedAddress('https://acme.tinycld.org')
    serve({ name: 'Acme' })
    expect((await fetchOrgInfo()).mailHost).toBe('')
    serve({ name: 'Acme', mailHost: 42 })
    expect((await fetchOrgInfo()).mailHost).toBe('')
})
