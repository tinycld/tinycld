// Compile-time only (not executed). Picked up by `tsc`. Mirrors
// ~/code/tinycld/new/spike2/neg.ts — the negative tripwire that proves
// useStore-style inference stays exact. If inference widens, the
// `@ts-expect-error` below becomes "unused" and `tsc` fails.
import { definePackageEntry, type MergeSchemas } from './config-types'

type ContactsSchema = {
    contacts: { type: { id: string; favorite: boolean }; relations: Record<string, never> }
}
const contactsEntry = definePackageEntry<ContactsSchema>()({
    manifest: { name: 'Contacts', slug: 'contacts', version: '0', description: '' },
    registerCollections: (nc, _core) => ({ contacts: nc('contacts') }),
})
const config = [contactsEntry] as const
type Merged = MergeSchemas<typeof config>

// positive: contacts key exists in the merged schema
const _hasKeys: keyof Merged extends never ? never : 'has-keys' = 'has-keys'
void _hasKeys

// the field type carries through
declare const c: Merged['contacts']['type']
const _fav: boolean = c.favorite
void _fav

// @ts-expect-error favorite is boolean, not string
const _bad: string = c.favorite
void _bad
