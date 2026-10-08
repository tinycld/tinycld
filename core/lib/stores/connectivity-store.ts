import { create } from '@tinycld/core/lib/store'

export interface ConnectivityState {
    isOnline: boolean
    isServerReachable: boolean
    /**
     * The latest request to our server failed at the network level and nothing
     * has succeeded since. Set on the first failure — unlike
     * `isServerReachable`, which waits for a sustained streak — so it covers a
     * load pbtsdb is retrying with backoff, whose failures land too far apart
     * to form a streak.
     */
    isRequestFailing: boolean
    setOnline: (online: boolean) => void
    setServerReachable: (reachable: boolean) => void
    setRequestFailing: (failing: boolean) => void
}

export const useConnectivityStore = create<ConnectivityState>()(set => ({
    isOnline: true,
    isServerReachable: true,
    isRequestFailing: false,
    setOnline: online => set({ isOnline: online }),
    setServerReachable: reachable => set({ isServerReachable: reachable }),
    setRequestFailing: failing => set({ isRequestFailing: failing }),
}))

export const selectIsOffline = (s: ConnectivityState) => !s.isOnline || !s.isServerReachable
