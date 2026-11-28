import * as fs from 'node:fs'
import * as path from 'node:path'
import type { CurrentPackage } from './discovery'

// Test files named before `--` (`tinycld-pkg test tests/a.test.ts`), each a
// path from the current directory or the workspace root, as test:e2e takes its
// spec files. Vitest runs in the package dir with the package's config, so each
// file becomes a filter relative to that dir; a file of another member would
// match nothing there, so it is refused.
export function resolveTestFiles(
    files: string[],
    opts: { cwd: string; workspaceRoot: string; pkg: CurrentPackage }
): string[] {
    const pkgDir = fs.realpathSync(opts.pkg.dir)
    return files.map(file => {
        const candidates = [path.resolve(opts.cwd, file), path.resolve(opts.workspaceRoot, file)]
        const found = candidates.find(candidate => fs.existsSync(candidate))
        if (!found) throw new Error(`Test file not found: ${file}`)
        const relative = path.relative(pkgDir, fs.realpathSync(found))
        if (relative.startsWith('..') || path.isAbsolute(relative)) {
            throw new Error(`Test file ${file} is not inside ${opts.pkg.name}`)
        }
        return relative
    })
}
