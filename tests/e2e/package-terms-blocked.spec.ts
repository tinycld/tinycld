import { expect, test } from '@playwright/test'
import { skipWithoutTermsStub, TERMS_STUB, termsSuperuser } from './package-terms-helpers'

// A package whose terms the owner has not accepted: what everybody sees.
//
// This file writes nothing. The termsstub package declares terms, and its seed
// records that the owner declined them, so on the shared e2e database the stub
// is closed for the whole run. The owner's decisions (accept, decline, the
// first launch, the setup wizard) are in package-terms-owner.spec.ts, on the
// second deployment.
//
// Not covered here: the gate is in the client only. The server answers a
// request for the stub's records and routes as for any other package, and no
// test here asserts a refusal.

test.describe('package terms: a package nobody may use yet', () => {
    test.skip(skipWithoutTermsStub)

    test('the server holds the terms version and text of the package', async () => {
        const pb = await termsSuperuser('primary')
        const row = await pb
            .collection('pkg_registry')
            .getFirstListItem(pb.filter('slug = {:slug}', { slug: TERMS_STUB.slug }))
        expect(row.terms_version).toBe(TERMS_STUB.version)
        expect(row.terms_accepted_version).toBe('')
        const terms = await pb
            .collection('pkg_terms')
            .getFirstListItem(pb.filter('pkg = {:slug}', { slug: TERMS_STUB.slug }))
        expect(terms.version).toBe(TERMS_STUB.version)
        expect(terms.body).toContain(TERMS_STUB.sentence)
    })
})
