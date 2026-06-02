import * as path from 'node:path'
import type { CurrentPackage } from './discovery'

export interface Command {
    bin: string
    args: string[]
    cwd: string
}

// A package's own tsconfig (feature) or the app's tsconfig (app shell).
function tsconfigFor(pkg: CurrentPackage): string {
    return path.join(pkg.dir, 'tsconfig.json')
}
function vitestConfigFor(pkg: CurrentPackage): string {
    return path.join(pkg.dir, 'vitest.config.ts')
}
function playwrightConfigFor(pkg: CurrentPackage): string {
    return path.join(pkg.dir, 'playwright.config.ts')
}

// `passthrough` are extra args forwarded verbatim to the underlying runner
// (e.g. `tinycld-pkg test:e2e -- -g "name" --workers=1`). They append after the
// fixed args so callers can filter to a single test, cap workers, etc.
export function buildTypecheckCommand(
    pkg: CurrentPackage,
    _appDir: string,
    passthrough: string[] = []
): Command {
    return { bin: 'tsc', args: ['--noEmit', '-p', tsconfigFor(pkg), ...passthrough], cwd: pkg.dir }
}

// Lint scoped to a single package's tree, using the app shell's biome
// config. We always run from the app dir (where biome.json lives) and
// target the package's dir as the argument — running biome from
// pkg.dir would miss app/biome.json and fall back to biome's defaults
// (which flag a much wider set of files and produce different output).
//
// Why this lives in `check` (and gates every package's per-PR CI):
// before this, biome only ran via `pnpm run lint` (the workspace-wide
// sweep) on app's CI. That meant feature packages could merge code
// with lint regressions, and the issue would only surface later on
// the next app PR — by which time the offending change had landed in
// multiple member repos. Scoping a lint pass into `tinycld-pkg check`
// catches each package's lint regressions on its OWN PR before merge.
export function buildLintCommand(
    pkg: CurrentPackage,
    appDir: string,
    passthrough: string[] = []
): Command {
    return {
        bin: 'biome',
        args: ['check', pkg.dir, ...passthrough],
        cwd: appDir,
    }
}

export function buildTestCommand(
    pkg: CurrentPackage,
    _appDir: string,
    passthrough: string[] = []
): Command {
    return {
        bin: 'vitest',
        args: ['run', '--config', vitestConfigFor(pkg), ...passthrough],
        cwd: pkg.dir,
    }
}

export function buildE2eCommand(
    pkg: CurrentPackage,
    _appDir: string,
    passthrough: string[] = []
): Command {
    return {
        bin: 'playwright',
        args: ['test', '--config', playwrightConfigFor(pkg), ...passthrough],
        cwd: pkg.dir,
    }
}

// check runs lint + typecheck + unit; passthrough only makes sense for one
// runner at a time, so it is intentionally NOT forwarded here (a combined
// run has no single target for a filter). Use `test` or `typecheck`
// directly to pass args. Order: lint first because it's the cheapest and
// catches issues that often surface as compile/test failures downstream.
export function buildCheckCommands(pkg: CurrentPackage, appDir: string): Command[] {
    return [
        buildLintCommand(pkg, appDir),
        buildTypecheckCommand(pkg, appDir),
        buildTestCommand(pkg, appDir),
    ]
}
