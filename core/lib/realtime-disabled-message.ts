/**
 * The rejection message the realtime guard uses (see realtime-enabled.ts).
 *
 * Its own module so `pocketbase.ts` can drop the resulting pbtsdb error reports
 * without importing realtime-enabled.ts, which imports `pb` from pocketbase.ts
 * and would close the cycle.
 */
export const REALTIME_DISABLED_MESSAGE = 'realtime is disabled for this page'
