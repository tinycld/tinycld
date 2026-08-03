// Promotes an exported dist/ into the prod-shaped releases layout the Go server
// reads. A TypeScript port of entrypoint.sh's promote_release():
//
//   <releasesDir>/_static/_expo/static/  ← dist/_expo/static/   (PoolAssets)
//   <releasesDir>/_static/assets/         ← dist/assets/          (PoolAssets)
//   <releasesDir>/<id>/app.html           ← dist/index.html       (SPA shell)
//   <releasesDir>/<id>/release-id.txt     ← <id>                  (VersionHandler)
//   <releasesDir>/current → <id>          (symlink)
//
// Extracted from e2e-serve.ts so BOTH e2e launchers promote through one code
// path. Two copies of this layout would drift, and a mismatch does not fail
// loudly — it presents as the server quietly handing the browser a stale bundle,
// so tests pass against JS that no longer matches the source.
//
// The releases dir is wiped each run: tests want exactly one release, not the
// cross-release asset pool production accumulates across deploys.

import * as fs from 'node:fs'
import * as path from 'node:path'

export function promoteRelease(
    distDir: string,
    releasesDir: string,
    releaseId: string,
    log: (msg: string) => void = () => {}
): void {
    log(`promoting dist/ → ${releasesDir} (current → ${releaseId})`)

    const indexHtml = path.join(distDir, 'index.html')
    if (!fs.existsSync(indexHtml)) {
        throw new Error(`${indexHtml} missing — expo export did not produce a web bundle`)
    }

    fs.rmSync(releasesDir, { recursive: true, force: true })
    const pool = path.join(releasesDir, '_static')
    fs.mkdirSync(path.join(pool, '_expo', 'static'), { recursive: true })
    fs.mkdirSync(path.join(pool, 'assets'), { recursive: true })

    // Merge each asset subtree into the pool if the export produced it.
    const expoStatic = path.join(distDir, '_expo', 'static')
    if (fs.existsSync(expoStatic)) {
        fs.cpSync(expoStatic, path.join(pool, '_expo', 'static'), { recursive: true })
    }
    const assets = path.join(distDir, 'assets')
    if (fs.existsSync(assets)) {
        fs.cpSync(assets, path.join(pool, 'assets'), { recursive: true })
    }

    const releaseDir = path.join(releasesDir, releaseId)
    fs.mkdirSync(releaseDir, { recursive: true })
    fs.copyFileSync(indexHtml, path.join(releaseDir, 'app.html'))
    fs.writeFileSync(path.join(releaseDir, 'release-id.txt'), releaseId)

    // current → <id>. A relative target keeps the symlink valid regardless of
    // where the releases dir is mounted.
    fs.symlinkSync(releaseId, path.join(releasesDir, 'current'))
}
