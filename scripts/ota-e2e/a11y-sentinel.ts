// Reads the iOS accessibility tree via `idb ui describe-all --json` and extracts
// the BundleSentinel element's bundle id (testID "ota-bundle-sentinel" → the iOS
// accessibilityIdentifier idb reports, label "bundle:<id>"). Asserting this id ==
// the new build-<ts>-ios proves the update is live ON SCREEN, not just executed.
//
// idb is an OPTIONAL dependency: when it is not installed, queryA11ySentinel
// returns null (skip), so the harness still runs on a machine without idb. Pure
// parser + injectable runner, mirroring the other ota-e2e modules.

import { spawn } from 'node:child_process'

interface A11yElement {
    AXIdentifier?: string
    identifier?: string
    AXLabel?: string
    label?: string
}

function elementId(el: A11yElement): string | undefined {
    return el.AXIdentifier ?? el.identifier
}
function elementLabel(el: A11yElement): string | undefined {
    return el.AXLabel ?? el.label
}

// Walk the flat element list, find ota-bundle-sentinel, parse bundle:<id> out of
// its label. Defensive against non-array input and missing fields.
export function findSentinelBundleId(tree: unknown): string | null {
    if (!Array.isArray(tree)) return null
    for (const el of tree as A11yElement[]) {
        if (elementId(el) === 'ota-bundle-sentinel') {
            const label = elementLabel(el) ?? ''
            const m = /^bundle:(.+)$/.exec(label)
            if (m) return m[1]
        }
    }
    return null
}

// Returns the raw `idb ui describe-all --json` stdout, or null if idb is not
// installed / not runnable (the optional-dependency skip path).
type IdbRunner = (udid: string) => Promise<string | null>

const realRunner: IdbRunner = udid =>
    new Promise<string | null>(resolve => {
        const child = spawn('idb', ['ui', 'describe-all', '--udid', udid, '--json'])
        let out = ''
        child.stdout.on('data', d => {
            out += d
        })
        // ENOENT (idb absent) or any spawn error -> skip (null), never throw.
        child.on('error', () => resolve(null))
        child.on('close', code => resolve(code === 0 ? out : null))
    })

// Query the a11y tree and extract the sentinel id. null means EITHER idb is
// unavailable OR the sentinel is not (yet) present — the caller distinguishes by
// retrying and, on a persistent null, logging a skip vs failing per the idb
// availability it was told about.
export async function queryA11ySentinel(
    udid: string,
    runner: IdbRunner = realRunner
): Promise<string | null> {
    const out = await runner(udid)
    if (out === null) return null
    try {
        return findSentinelBundleId(JSON.parse(out))
    } catch {
        return null
    }
}

// Whether idb is on PATH — lets the runner decide skip (absent) vs fail (present
// but no sentinel). Spawns `idb --help` once.
export function idbAvailable(): Promise<boolean> {
    return new Promise<boolean>(resolve => {
        const child = spawn('idb', ['--help'])
        child.on('error', () => resolve(false))
        child.on('close', code => resolve(code === 0))
    })
}
