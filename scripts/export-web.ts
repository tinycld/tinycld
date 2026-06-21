// Builds the static web bundle (dist/) the way the e2e static-serve flow and
// production both want it: a single `expo export --platform web` with external
// sourcemaps. Extracted so the build runs in exactly ONE place — invoked both
// by scripts/e2e-serve.ts (when it isn't reusing an existing dist/) and by the
// shared CI action (.github/actions/configure-test-env), which pre-builds the
// bundle once for every package's e2e job.
//
// Usage: tsx scripts/export-web.ts [--release-id <id>]
//
// --source-maps external emits a .js.map next to each chunk (+ a
// sourceMappingURL); the Go server serves them via the /_expo/static pool route,
// so a minified error maps back to its original .tsx (Sentry symbolicates with
// them server-side; AppErrorBoundary symbolicates client-side; the sourcemaps
// e2e spec verifies they're present + correct).

import { spawnSync } from 'node:child_process'
import * as path from 'node:path'

const ROOT = path.resolve(import.meta.dirname, '..')

function flagValue(name: string): string | null {
    const i = process.argv.indexOf(name)
    if (i === -1) return null
    const v = process.argv[i + 1]
    if (v === undefined || v.startsWith('-')) {
        throw new Error(`export-web: flag ${name} requires a value`)
    }
    return v
}

// EXPO_PUBLIC_RELEASE_ID is inlined into the bundle for the in-app version
// poll; a caller (e2e-serve) that promotes the bundle passes the SAME id it
// writes to release-id.txt so boot-id == served-id. When unset it's cosmetic
// (the poll just has nothing to compare), so a standalone build is fine without.
export function exportWeb(releaseId?: string): void {
    const env: NodeJS.ProcessEnv = {
        ...process.env,
        EXPO_PUBLIC_ENV: 'web',
        // E2E-only build tweaks (this script builds the e2e bundle; production
        // is built by the Dockerfile's own `expo export`). Shrink @tinycld/text's
        // suggestion-grouping idle window from 30s to 1s so the bulk-resolve
        // spec can mint distinct suggestions in seconds instead of sleeping out
        // two 30s windows. Honor a caller-set value if present.
        EXPO_PUBLIC_TEXT_SUGGESTION_IDLE_MS:
            process.env.EXPO_PUBLIC_TEXT_SUGGESTION_IDLE_MS ?? '1000',
        // The full-graph cold export can exceed Node's default old-space ceiling;
        // match dev.ts / the EAS build at 8GB. Preserve a caller-set value.
        NODE_OPTIONS: process.env.NODE_OPTIONS?.includes('--max-old-space-size')
            ? process.env.NODE_OPTIONS
            : `${process.env.NODE_OPTIONS ?? ''} --max-old-space-size=8192`.trim(),
    }
    if (releaseId) env.EXPO_PUBLIC_RELEASE_ID = releaseId

    const result = spawnSync(
        'pnpm',
        ['exec', 'expo', 'export', '--platform', 'web', '--source-maps', 'external'],
        { cwd: ROOT, stdio: 'inherit', env }
    )
    if (result.status !== 0) {
        throw new Error(`expo export exited ${result.status}`)
    }
}

// Run directly (the CI action invokes `tsx scripts/export-web.ts`). When
// imported (by e2e-serve), this guard keeps it from double-building.
if (import.meta.url === `file://${process.argv[1]}`) {
    exportWeb(flagValue('--release-id') ?? undefined)
}
