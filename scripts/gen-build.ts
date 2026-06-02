import { execFileSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as path from 'node:path'

// Resolve a manifest `build.script` value to a concrete on-disk file. Prefers
// an exports-map entry; falls back to <script>.{ts,mjs,js} then the bare path.
export function resolveBuildScriptPath(packageDir: string, script: string): string {
    const pkgJsonPath = path.join(packageDir, 'package.json')
    const exportsMap: Record<string, string> = fs.existsSync(pkgJsonPath)
        ? (JSON.parse(fs.readFileSync(pkgJsonPath, 'utf-8')).exports ?? {})
        : {}
    const exportTarget = exportsMap[`./${script}`]
    if (typeof exportTarget === 'string') {
        const abs = path.join(packageDir, exportTarget.replace(/^\.\//, ''))
        if (fs.existsSync(abs)) return abs
    }
    for (const ext of ['.ts', '.mjs', '.js']) {
        const candidate = path.join(packageDir, `${script}${ext}`)
        if (fs.existsSync(candidate)) return candidate
    }
    const bare = path.join(packageDir, script)
    if (fs.existsSync(bare)) return bare
    throw new Error(
        `Package build script not found: tried ./${script}.{ts,mjs,js} and ./${script} under ${packageDir}`
    )
}

// Locate the workspace tsx binary (members have no node_modules of their own;
// tsx is hoisted to the workspace-root or one level up in CI layouts).
function tsxBinary(wsRoot: string): string {
    for (const c of [
        path.join(wsRoot, 'node_modules/.bin/tsx'),
        path.join(wsRoot, '..', 'node_modules/.bin/tsx'),
    ]) {
        if (fs.existsSync(c)) return c
    }
    return 'tsx'
}

export interface BuildPkg {
    packageName: string
    packageDir: string
    script: string
}

// Run each package's build script once (synchronous). Used by the generator
// BEFORE emitting config so build outputs (e.g. text's editorHtml.ts) exist for
// the subsequent typecheck/bundle.
export function runPackageBuilds(wsRoot: string, builds: BuildPkg[]): void {
    if (builds.length === 0) return
    const tsx = tsxBinary(wsRoot)
    // Package build scripts (e.g. text's webview-editor bundler) point esbuild
    // at the app shell's node_modules to resolve the deps they bundle (@tiptap,
    // yjs, react, …). Pass the app dir explicitly so they don't fall back to a
    // guessed relative path — under pnpm those deps are scoped to app/node_modules
    // (not flat-hoisted to the workspace root), so the path must be exact.
    const appDir = path.join(wsRoot, 'app')
    for (const b of builds) {
        const scriptPath = resolveBuildScriptPath(b.packageDir, b.script)
        console.log(
            `[generate] running build for ${b.packageName}: ${path.relative(wsRoot, scriptPath)}`
        )
        execFileSync(tsx, [scriptPath], {
            cwd: b.packageDir,
            stdio: 'inherit',
            env: { ...process.env, TINYCLD_APP_ROOT: appDir },
        })
    }
}
