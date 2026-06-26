// Shared "update is live" assertion for both ota-e2e runners. After a healthy
// flip, prove the NEW bundle's JS actually EXECUTED + mounted — not just that the
// native currentId flipped. The running bundle POSTs a boot beacon (BundleSentinel
// → POST /api/app/boot) once its real provider tree commits; the server logs
// `app-boot: rendered` with the bundle id, which we read from _logs here. This is
// the proof that works in a RELEASE build, where console.log is not observable and
// the opacity:0 a11y sentinel isn't surfaced to idb (both were tried and failed on
// a real run). The caller passes its own fail(msg): never + log(msg) so the proof
// logic lives in one place while each runner keeps its output prefix.
import { fetchBootBeaconIds } from './boot-beacon-poller'
import { pollForBundleId } from './logs-poller'

export async function assertUpdateIsLive(
    serverUrl: string,
    token: string,
    newId: string,
    fail: (msg: string) => never,
    log: (msg: string) => void
): Promise<void> {
    try {
        await pollForBundleId({
            fetchCurrentIds: () => fetchBootBeaconIds(serverUrl, token),
            target: newId,
            timeoutMs: 60_000,
            intervalMs: 3_000,
            onPoll: ids => {
                if (ids.length) log(`  boot beacons seen: ${ids.join(', ')}`)
            },
        })
        // Inside the try so the proof is logged ONLY on success — not relying on
        // fail()'s never-return to keep it off the failure path.
        log(`boot-beacon proof: new bundle JS executed + mounted (id=${newId})`)
    } catch {
        fail(
            `boot-beacon proof missing: the app never POSTed an 'app-boot: rendered' beacon for ` +
                `id=${newId} within 60s. The new bundle's JS did not execute/mount (the real provider ` +
                `tree never committed) even though the native currentId flipped — OR the app couldn't ` +
                `reach the server to report. Check the device + server _logs.`
        )
    }
}
