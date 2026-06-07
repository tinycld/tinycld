// Compile-time only (not executed). Picked up by `tsc`. Mirrors
// ~/code/tinycld/new/spike2/neg.ts — the negative tripwire that proves
// useStore-style inference stays exact. If inference widens, the
// `@ts-expect-error` below becomes "unused" and `tsc` fails.
import { definePackageEntry, type MergeSchemas, type PackageStoresReturn } from './config-types'

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

// Lean-shell guarantee: an EMPTY config must yield a spreadable store map
// (Record<string, never>), NOT `unknown`. Regression guard for the 0-feature
// typecheck (PackageStoresReturn<readonly []> must be spreadable).
type EmptyStores = PackageStoresReturn<readonly []>
// must be an object type (spreadable) — assignable to Record<string, never>
const _emptyStoresOk: Record<string, never> = {} as EmptyStores
void _emptyStoresOk
// and spreading it must be legal (the actual failure mode)
const _spreadOk = { ...({} as EmptyStores) }
void _spreadOk
