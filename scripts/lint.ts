import { spawnSync } from 'node:child_process'
import * as path from 'node:path'
import { getPackages } from '../../tinycld.packages'
import { APP_DIR, memberDir } from './paths'

// Run biome over the app shell, core, and every INSTALLED member.
//
// The member list is derived from getPackages() — the same discovery the
// generator uses — rather than hardcoded. A hardcoded list breaks the
// partial-assembly guarantee: a workspace assembled with only some features
// (or one with members parked) made biome fail with an opaque
// "No such file or directory (os error 2)" internal error for each absent dir.
//
// `.` and `core` cover the app shell and @tinycld/core, so core is filtered out
// of the derived list to avoid linting it twice.
//
// Pass --write to apply fixes (pnpm run lint:fix).
const write = process.argv.includes('--write')

const members = getPackages()
    .filter(name => name !== '@tinycld/core')
    .map(name => path.relative(APP_DIR, memberDir(name)))

const args = ['check', ...(write ? ['--write'] : []), '.', 'core', ...members]
const result = spawnSync('biome', args, { cwd: APP_DIR, stdio: 'inherit' })

process.exit(result.status ?? 1)
