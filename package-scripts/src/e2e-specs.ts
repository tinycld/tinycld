import * as fs from 'node:fs'
import * as path from 'node:path'
import { packageForDir } from './discovery'
import { type MemberSpecs, SPECS_ENV } from './multi-member'
import type { Command } from './runners'

export const SKIP_EXPORT_ENV = { TINYCLD_E2E_SKIP_EXPORT: '1' }

function resolveSpecFile(file: string, cwd: string, workspaceRoot: string): string {
    const candidates = [path.resolve(cwd, file), path.resolve(workspaceRoot, file)]
    const found = candidates.find(candidate => fs.existsSync(candidate))
    if (!found) throw new Error(`Spec file not found: ${file}`)
    return fs.realpathSync(found)
}

// Group spec files by the member that owns them, in first-seen order, each
// file as its real absolute path.
export function groupSpecFiles(
    files: string[],
    opts: { cwd: string; workspaceRoot: string; appDir: string }
): MemberSpecs[] {
    const groups = new Map<string, MemberSpecs>()
    for (const file of files) {
        const absolute = resolveSpecFile(file, opts.cwd, opts.workspaceRoot)
        const pkg = packageForDir(path.dirname(absolute), opts.appDir)
        if (!pkg) throw new Error(`Spec file is not inside a workspace member: ${file}`)
        if (!fs.existsSync(path.join(pkg.dir, 'playwright.config.ts'))) {
            throw new Error(`${pkg.name} has no playwright.config.ts (spec file ${file})`)
        }
        const group = groups.get(pkg.dir) ?? { dir: pkg.dir, name: pkg.name, files: [] }
        group.files.push(absolute)
        groups.set(pkg.dir, group)
    }
    return [...groups.values()]
}

// A named spec file must run: without this, a stub-dependent spec skips outside
// CI, and a run of only skipped tests reports green. tests/e2e/helpers.ts reads it.
export const REQUIRE_STUBS_ENV = { TINYCLD_E2E_REQUIRE_STUBS: '1' }

// ONE Playwright run for all the members' files, through the combined config:
// each e2e run starts the servers, builds the web export and seeds the
// fixtures, so a run per member (or per file) pays for all of that again.
export function buildSpecFilesCommand(
    members: MemberSpecs[],
    appDir: string,
    passthrough: string[] = []
): Command {
    return {
        bin: 'playwright',
        args: [
            'test',
            '--config',
            path.join(import.meta.dirname, 'multi-member.config.ts'),
            ...passthrough,
        ],
        cwd: appDir,
        env: { [SPECS_ENV]: JSON.stringify(members), ...REQUIRE_STUBS_ENV },
    }
}
