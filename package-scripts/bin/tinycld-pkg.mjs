#!/usr/bin/env node
// Launcher for the tinycld-pkg CLI.
//
// The CLI itself (src/cli.ts) is TypeScript, so it must run under tsx. We can't
// rely on a `#!/usr/bin/env tsx` shebang: under pnpm a consuming member only
// gets the bins of its *direct* deps linked into node_modules/.bin, and tsx is
// a dep of THIS package — not of the member — so `tsx` isn't on the member's
// PATH. Instead we resolve tsx's CLI from this package's own node_modules and
// hand it the TS entry, which works regardless of package manager or hoisting.
import { spawn } from 'node:child_process'
import { createRequire } from 'node:module'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const cliEntry = resolve(here, '../src/cli.ts')

const require = createRequire(import.meta.url)
const tsxCli = require.resolve('tsx/cli')

const child = spawn(process.execPath, [tsxCli, cliEntry, ...process.argv.slice(2)], {
    stdio: 'inherit',
})
child.on('exit', code => process.exit(code ?? 0))
child.on('error', err => {
    console.error('[tinycld-pkg] failed to launch:', err)
    process.exit(1)
})
