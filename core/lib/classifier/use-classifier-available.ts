import { useQuery } from '@tanstack/react-query'
import { pb } from '@tinycld/core/lib/pocketbase'

type StatusResponse = { available: boolean }

// Members cannot read system_settings, so availability comes from a route
// that exposes only whether the feature is on.
export function useClassifierAvailable() {
    const { data, isLoading } = useQuery({
        queryKey: ['classifier', 'status'],
        queryFn: async (): Promise<StatusResponse> => await pb.send('/api/classifier/status', {}),
    })
    return { available: data?.available ?? false, isLoading }
}
