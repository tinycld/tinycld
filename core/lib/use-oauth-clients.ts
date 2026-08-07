import { useQuery, useQueryClient } from '@tanstack/react-query'
import { captureException } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb } from '@tinycld/core/lib/pocketbase'

// The projection GET /oauth/clients returns — mirrors AdminClientView in
// core/server/oauth/clients_endpoint.go. Deliberately does NOT include
// client_secret_hash: the server never sends it, and declaring it here would
// invite a future reader to think it might arrive.
export interface OAuthClient {
    id: string
    client_id: string
    name: string
    type: 'public' | 'confidential'
    scopes: string
    is_first_party: boolean
    disabled: boolean
    active_grants: number
}

const CLIENTS_QUERY_KEY = ['oauth-clients']

// oauth_clients is superuser-only at the collection level (every API rule is
// null), so this cannot be a useOrgLiveQuery — there is no readable collection
// to subscribe to. The Go endpoint is the only way in, which also means the
// list does not update itself: refetch after a mutation is what keeps it
// honest.
// `enabled` gates the request itself, not just the render: the endpoint 403s a
// non-admin, so firing it for one would produce a load error on a screen they
// were never entitled to see. Also holds the query until the caller's role has
// settled, so a cold-load admin does not fire a doomed request.
export function useOAuthClients(enabled = true) {
    return useQuery<OAuthClient[]>({
        queryKey: CLIENTS_QUERY_KEY,
        enabled,
        queryFn: async () => {
            const body = await pb.send<{ clients?: OAuthClient[] }>('/oauth/clients', {
                method: 'GET',
            })
            return body.clients ?? []
        },
    })
}

// useSetClientDisabled flips the kill switch.
//
// Sends the DESIRED state rather than a toggle so the request is idempotent —
// two admins acting on the same stale list converge instead of undoing each
// other. Invalidates the list on settle (not just on success) because a failed
// write still leaves the local list possibly out of step with the server.
export function useSetClientDisabled() {
    const queryClient = useQueryClient()

    return useMutation({
        mutationFn: async ({ id, disabled }: { id: string; disabled: boolean }) => {
            await pb.send(`/oauth/clients/${id}/disabled`, {
                method: 'POST',
                body: { disabled },
            })
        },
        onError: err => captureException('oauth.adminClients.setDisabled', err),
        onSettled: () => queryClient.invalidateQueries({ queryKey: CLIENTS_QUERY_KEY }),
    })
}
