// Shared .env loader for ecosystem CLI scripts (db reset/seed, smoke tests, …).
//
// Credentials and other dev config (ADMIN_USER_*, TEST_USER_*, REVIEW_DEMO_*,
// SMOKE_TEST_*, …) live in the workspace-root .env — `~/code/tinycld/.env` —
// which sits one level *above* the app-shell member and two or more levels
// above a sibling's scripts. A bare `process.loadEnvFile()` only looks at the
// current working directory, so every script was missing the root file. This
// helper walks up from the cwd, loading each `.env` it finds.
//
// `process.loadEnvFile` never overwrites an already-set variable, so the load
// order here IS the precedence order: a real env var (shell / CI / a parent
// process that already exported it) wins, then the nearest `.env` to the cwd,
// then each ancestor `.env` up to the workspace root. Every file is optional —
// none exist in CI, which sets the variables directly.
//
// Node-only (uses node:fs / node:path / process.loadEnvFile). Import it from
// scripts and tooling, never from app/runtime code.

import { existsSync } from 'node:fs'
import { dirname, join, parse } from 'node:path'

export interface LoadEnvOptions {
    // Directory to start the upward search from. Defaults to process.cwd().
    cwd?: string
    // How many ancestor directories above `cwd` to scan (cwd itself always
    // counts). The default of 4 reaches the workspace root from the deepest
    // script location we have (a sibling's `tinycld/<slug>/scripts/…`) while
    // still stopping well before the filesystem root.
    maxLevels?: number
}

// Load every `.env` found walking up from `cwd`, nearest-first. Returns the
// absolute paths that were actually loaded (handy for logging / tests).
export function loadEnv(options: LoadEnvOptions = {}): string[] {
    const { cwd = process.cwd(), maxLevels = 4 } = options
    const loaded: string[] = []

    let dir = cwd
    const fsRoot = parse(dir).root
    for (let level = 0; level <= maxLevels; level++) {
        const envPath = join(dir, '.env')
        if (existsSync(envPath)) {
            try {
                process.loadEnvFile(envPath)
                loaded.push(envPath)
            } catch {
                // Unreadable / malformed — skip it rather than crash the script.
            }
        }
        if (dir === fsRoot) break
        dir = dirname(dir)
    }

    return loaded
}
