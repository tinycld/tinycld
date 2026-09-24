import { type CollectionImpl, eq, type UtilsRecord } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { newRecordId } from 'pbtsdb/core'
import type { GroupGrant, GroupGrantRow, GroupRoleOption, NewGroupGrant } from './types'

export interface UseGroupGrantsOptions<
    Role extends string,
    Row extends GroupGrantRow<Role>,
    TKey extends string | number,
    TUtils extends UtilsRecord,
    TInsert extends GroupGrantRow<Role>,
> {
    collection: CollectionImpl<Row, TKey, TUtils, never, TInsert>
    // Pins Role so buildRow and the role menu agree on the union.
    roles: readonly GroupRoleOption<Role>[]
    isForResource: (row: Row) => boolean
    buildRow: (grant: NewGroupGrant<Role>) => TInsert
}

/**
 * Query + mutations for the group grants on one resource of a package's
 * membership table. Generic over the package's collection, so core never
 * names a package table: the package passes its collection, a predicate that
 * picks the resource, and a builder that turns a core grant into its full
 * insert row. The result spreads straight into GroupShareSection.
 *
 * The query fetches grant rows (user = '') for the whole collection and the
 * predicate narrows to the resource in render. A grant table is small: one
 * row per (resource, group), and the caller's rules already confine what it
 * can read.
 */
export function useGroupGrants<
    Role extends string,
    Row extends GroupGrantRow<Role>,
    TKey extends string | number,
    TUtils extends UtilsRecord,
    TInsert extends GroupGrantRow<Role>,
>({
    collection,
    isForResource,
    buildRow,
}: UseGroupGrantsOptions<Role, Row, TKey, TUtils, TInsert>) {
    const { data: rows, isReady } = useLiveQuery(
        query => query.from({ grant: collection }).where(({ grant }) => eq(grant.user, '')),
        [collection]
    )

    const grants: GroupGrant<Role>[] = (rows ?? [])
        .filter(row => isForResource(row))
        .map(row => ({ grantId: row.id, groupId: row.group, role: row.role }))

    const add = useMutation<void, Error, { groupId: string; role: Role }>({
        mutationFn: mutation(function* ({ groupId, role }) {
            yield collection.insert(buildRow({ id: newRecordId(), user: '', group: groupId, role }))
        }),
    })
    const changeRole = useMutation<void, Error, { grantId: TKey; role: Role }>({
        mutationFn: mutation(function* ({ grantId, role }) {
            // draft is WritableDeep<TInsert>, a deferred type, so a direct
            // property write does not typecheck against Role.
            yield collection.update(grantId, draft => {
                Object.assign(draft, { role })
            })
        }),
    })
    const remove = useMutation<void, Error, TKey>({
        mutationFn: mutation(function* (grantId) {
            yield collection.delete(grantId)
        }),
    })

    return {
        grants,
        isReady,
        onAdd: (groupId: string, role: Role) => add.mutate({ groupId, role }),
        onRoleChange: (grantId: TKey, role: Role) => changeRole.mutate({ grantId, role }),
        onRemove: (grantId: TKey) => remove.mutate(grantId),
        isPending: add.isPending || changeRole.isPending || remove.isPending,
    }
}
