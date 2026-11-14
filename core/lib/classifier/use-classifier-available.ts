import { useQuery } from '@tanstack/react-query'
import { pb } from '@tinycld/core/lib/pocketbase'

type StatusResponse = { available: boolean }

// Members cannot read system_settings, so availability comes from a route
// that exposes only whether the feature is on. `isKnown` is false while the
// request runs and after it fails, so a screen can show neither state then
// instead of reading a failed request as "unavailable".
export function useClassifierAvailable() {
    const { data, isSuccess } = useQuery({
        queryKey: ['classifier', 'status'],
        queryFn: async (): Promise<StatusResponse> => await pb.send('/api/classifier/status', {}),
    })
    return { available: data?.available ?? false, isKnown: isSuccess }
}
