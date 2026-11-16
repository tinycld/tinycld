#!/usr/bin/env tsx
import { type CurrentPackage, discover } from './discovery'
import { buildSpecFilesCommand, groupSpecFiles, SKIP_EXPORT_ENV } from './e2e-specs'
import { resolveTopic } from './help-screenshots'
import { runAll } from './orchestrator'
import {
    buildCheckCommands,
    buildE2eCommand,
    buildHelpScreenshotsCommand,
    buildTestCommand,
    buildTypecheckCommand,
    type Command,
} from './runners'
import { runCommand } from './spawn'
import { allTargets, type Verb } from './targets'

function commandsFor(
    verb: Verb,
    pkg: CurrentPackage,
    appDir: string,
    passthrough: string[],
    ownArgs: string[] = []
): Command[] {
    switch (verb) {
        case 'typecheck':
            return [buildTypecheckCommand(pkg, appDir, passthrough)]
        case 'test':
            return [buildTestCommand(pkg, appDir, passthrough)]
        case 'test:e2e':
            return [buildE2eCommand(pkg, appDir, passthrough)]
        case 'check':
            return buildCheckCommands(pkg, appDir)
        case 'help:screenshots': {
            const resolved = resolveTopic(ownArgs, passthrough)
            return [buildHelpScreenshotsCommand(pkg, appDir, resolved)]
        }
        default:
            throw new Error(
                `Unknown verb '${verb}'. Use one of: typecheck | test | test:e2e | check | help:screenshots`
            )
    }
}

async function runForPackage(
    verb: Verb,
    pkg: CurrentPackage,
    appDir: string,
    passthrough: string[],
    ownArgs: string[] = [],
    env: Record<string, string> = {}
): Promise<number> {
    for (const cmd of commandsFor(verb, pkg, appDir, passthrough, ownArgs)) {
        const code = await runCommand({ ...cmd, env: { ...cmd.env, ...env } })
        if (code !== 0) return code
    }
    return 0
}

async function main() {
    const argv = process.argv.slice(2)
    const [verb, ...rest] = argv as [Verb, ...string[]]
    if (!verb) {
        console.error(
            'usage: tinycld-pkg <typecheck|test|test:e2e|check|help:screenshots> [--all] [--bail] [-- <runner args>]\n' +
                '       tinycld-pkg test:e2e <spec file>... [--skip-export] [-- <playwright args>]'
        )
        process.exit(2)
    }
    // Everything after a `--` separator is forwarded verbatim to the underlying
    // runner (vitest/playwright/tsc) — e.g. `test:e2e -- -g "name" --workers=1`
    // to filter to one test. Args before `--` are tinycld-pkg's own flags.
    const sepIndex = rest.indexOf('--')
    const ownArgs = sepIndex === -1 ? rest : rest.slice(0, sepIndex)
    const passthrough = sepIndex === -1 ? [] : rest.slice(sepIndex + 1)
    const all = ownArgs.includes('--all')
    const bail = ownArgs.includes('--bail')
    const skipExport = ownArgs.includes('--skip-export')
    // Spec files named before `--`, from any members: one Playwright run (one
    // server start, one export, one seed) for the whole call.
    const specFiles = verb === 'test:e2e' ? ownArgs.filter(arg => !arg.startsWith('--')) : []
    if (skipExport && verb !== 'test:e2e') {
        console.error('--skip-export applies to test:e2e only')
        process.exit(2)
    }
    if (all && specFiles.length > 0) {
        console.error('Cannot combine --all with spec files.')
        process.exit(2)
    }

    if (all && passthrough.length > 0) {
        console.error(
            'Cannot combine --all with `-- <runner args>`: passthrough targets a single package.'
        )
        process.exit(2)
    }
    if (all && verb === 'help:screenshots') {
        console.error('help:screenshots runs one package at a time')
        process.exit(2)
    }
    const { workspaceRoot, appDir, currentPackage } = discover()

    if (all) {
        const targets = allTargets(workspaceRoot, appDir, verb)
        const result = await runAll(
            targets.map(t => t.name),
            async name => {
                const pkg = targets.find(t => t.name === name)!
                console.log(`\n=== ${verb} ${name} ===`)
                return runForPackage(verb, pkg, appDir, [])
            },
            { bail }
        )
        const summary = result.results
            .map(r => `${r.code === 0 ? '✓' : '✗'} ${r.target}`)
            .join('  ')
        console.log(`\n${summary}`)
        process.exit(result.exitCode)
    }

    if (specFiles.length > 0) {
        const members = groupSpecFiles(specFiles, { cwd: process.cwd(), workspaceRoot, appDir })
        const cmd = buildSpecFilesCommand(members, appDir, passthrough)
        const env = skipExport ? { ...cmd.env, ...SKIP_EXPORT_ENV } : cmd.env
        process.exit(await runCommand({ ...cmd, env }))
    }

    if (!currentPackage) {
        console.error('Not inside a package or the app shell.')
        process.exit(2)
    }
    const env = skipExport ? SKIP_EXPORT_ENV : {}
    const code = await runForPackage(verb, currentPackage, appDir, passthrough, ownArgs, env)
    process.exit(code)
}

main().catch((error: unknown) => {
    console.error(error instanceof Error ? error.message : String(error))
    process.exit(2)
})
