// Reads the device console for the BundleSentinel boot line
// (core/lib/bundle-sentinel.tsx -> bootLogLine), which the app logs ONLY after
// the real provider tree commits. Finding it with the new build-<ts>-ios id is
// proof the new bundle's JS executed and mounted — stronger than the native
// currentId flip. Pure parser + an injectable spawn for deterministic tests,
// mirroring logs-poller.ts / bad-bundle-poller.ts.

import { spawn } from 'node:child_process'

// Parse the LAST `[tinycld] app-boot: rendered bundle id=<id> hash=<...>` line's
// id from a console blob (most recent boot wins). null when absent.
export function extractBootBundleId(logText: string): string | null {
    const re = /\[tinycld\] app-boot: rendered bundle id=(\S+) hash=/g
    let last: string | null = null
    for (const m of logText.matchAll(re)) {
        last = m[1]
    }
    return last
}

type SpawnFn = (udid: string, sinceSeconds: number) => Promise<string>

// Default spawn: `simctl log show` filtered to the boot line over the last N
// seconds, compact style. log show (not `log stream`) returns and exits, which is
// what we want for a one-shot scrape.
const realSpawn: SpawnFn = (udid, sinceSeconds) =>
    new Promise<string>((resolve, reject) => {
        const child = spawn('xcrun', [
            'simctl',
            'spawn',
            udid,
            'log',
            'show',
            '--predicate',
            'eventMessage CONTAINS "app-boot: rendered"',
            '--last',
            `${sinceSeconds}s`,
            '--style',
            'compact',
        ])
        let out = ''
        let err = ''
        child.stdout.on('data', d => {
            out += d
        })
        child.stderr.on('data', d => {
            err += d
        })
        child.on('error', reject)
        child.on('close', code => {
            // log show exits non-zero on some transient conditions; surface stderr
            // but still resolve with whatever stdout we captured so the caller's
            // retry loop can decide (an empty string parses to null).
            if (code !== 0 && out === '')
                reject(new Error(`simctl log show exited ${code}: ${err}`))
            else resolve(out)
        })
    })

// Scrape once and parse. Returns the boot bundle id, or null if no boot line was
// captured in the window. The caller retries.
export async function scrapeBootBundleId(
    udid: string,
    sinceSeconds: number,
    spawnFn: SpawnFn = realSpawn
): Promise<string | null> {
    const out = await spawnFn(udid, sinceSeconds)
    return extractBootBundleId(out)
}
