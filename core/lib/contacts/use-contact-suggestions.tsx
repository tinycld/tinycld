import type { Collection } from '@tanstack/db'
import { eq } from '@tanstack/db'
import { usePackages } from '@tinycld/core/lib/packages/use-packages'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import type { ReactNode } from 'react'

/**
 * Minimum shape every consumer of contact suggestions needs. Lives in core
 * so packages can render their own pickers without importing from
 * @tinycld/contacts directly (which would make the dep load-bearing at
 * compile time and break the lean-shell guarantee).
 */
export interface ContactSuggestion {
    id: string
    first_name: string
    last_name: string
    email: string
}

/**
 * Minimal local shape of a `contacts` record needed to run the suggestions
 * query. The `contacts` package owns the real schema; core only declares the
 * fields it reads so the soft dependency stays compile-time optional (see the
 * lean-shell guarantee in the ecosystem docs).
 */
interface ContactRecord extends ContactSuggestion {
    owner: string
}

interface ContactSuggestionsProps {
    /**
     * Receives the contact list whenever the underlying live query updates.
     * Returns whatever JSX (or null) the consumer wants to render — typically
     * a filtered dropdown of suggestions.
     */
    children: (contacts: ContactSuggestion[]) => ReactNode
}

/**
 * Subscribes to the current user_org's contacts and renders via a
 * children-as-function. Splits the runtime gate (is the contacts package
 * even linked?) from the data subscription so we never call
 * `useStore('contacts')` when the collection isn't registered — which would
 * throw synchronously.
 *
 * Render-prop instead of a plain hook because `useStore('contacts')` cannot
 * be conditionally called; isolating the subscription inside a child
 * component is the cleanest way to obey the rules of hooks while still
 * letting the parent render nothing when contacts is absent.
 */
export function ContactSuggestionsProvider({ children }: ContactSuggestionsProps) {
    const packages = usePackages()
    const isContactsLinked = packages.some(p => p.slug === 'contacts')
    if (!isContactsLinked) return null
    return <ActiveContactSuggestions>{children}</ActiveContactSuggestions>
}

function ActiveContactSuggestions({ children }: ContactSuggestionsProps) {
    // `contacts` isn't in core's store map (it belongs to the contacts
    // package), so the key + result are bridged through `unknown` to the
    // minimal ContactRecord collection this query needs.
    const [contactsCollection] = useStore('contacts' as unknown as never) as unknown as [
        Collection<ContactRecord>,
    ]

    const { data } = useOrgLiveQuery(
        (query, { userId }) =>
            query
                .from({ contacts: contactsCollection })
                .where(({ contacts }) => eq(contacts.owner, userId))
                .orderBy(({ contacts }) => contacts.first_name, 'asc'),
        []
    )

    return <>{children((data as ContactSuggestion[] | undefined) ?? [])}</>
}
