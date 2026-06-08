import * as fs from 'node:fs'
import * as path from 'node:path'
import { getPackages } from '../../tinycld.packages'
import { manifestToConfigPkg, validateSidebarContributions } from './describe-packages'
import { type BuildPkg, runPackageBuilds } from './gen-build'
import { buildConfigSource, buildSeedsSource, type ConfigPkg } from './gen-config'
import { buildHelpSource, type HelpGroupInput, parseFrontmatter } from './gen-help'
import { buildPackageIconsSource } from './gen-icons'
import { emitPublicRoutes, emitRoutes } from './gen-routes'
import {
    buildBundledPackages,
    buildGoWork,
    buildMemberGoWork,
    buildPackageExtensionsGo,
    replaceSymlink,
    type ServerPkg,
} from './gen-server'
import { buildUniwindSources, type UniwindSource } from './gen-uniwind'
import { loadManifest, type PackageManifest } from './load-manifest'
import {
    APP_DIR,
    GENERATED_DIR,
    HOOKS_DIR,
    MIGRATIONS_DIR,
    memberDir,
    PUBLIC_ROUTES_BASE,
    ROUTES_BASE,
    SERVER_DIR,
    WS_ROOT,
} from './paths'

// Resolve a package.json exports subpath to a directory relative to packageDir.
// e.g. exports['./screens/*'] === './tinycld/contacts/screens/*.tsx'
// → returns 'tinycld/contacts/screens' for subpath 'screens'.
function resolveExportDir(packageDir: string, subpath: string): string | null {
    const pkgJson = JSON.parse(fs.readFileSync(path.join(packageDir, 'package.json'), 'utf8'))
    const exp = pkgJson.exports?.[`./${subpath}/*`]
    if (typeof exp !== 'string') return null
    // strip leading './' and trailing '/*.<ext>'
    return exp.replace(/^\.\//, '').replace(/\/\*\.[^.]+$/, '')
}

// Write the workspace-root biome.json. Biome only searches UPWARD for config,
// and the canonical biome.json lives at <ws-root>/tinycld/ — a SIBLING of the
// feature members, never an ancestor. Without a root config, running biome from
// inside a member (or via the editor/LSP) finds nothing and falls back to
// biome's built-in defaults (wrong indent/quotes/semicolons), flooding output
// with bogus reformatting. This minimal root config makes the canonical config
// resolvable from anywhere: it's the one `root: true` config, and it `extends`
// the canonical one (which is `root: false`). Members may drop their own
// `root: false` biome.json that extends canonical to override rules; most don't.
//
// Written every install (not just on bootstrap assemble) so it self-heals: the
// canonical config's `root: false` ships via the tinycld repo, and a non-root
// config with no root above it breaks `pnpm run lint` — emitting this here means
// the same install that pulls `root: false` also lays down the root config.
// Bootstrap-owned (gitignored at the ws-root); content is static, so always
// rewrite.
//
// `vcs.root` points at the app member dir (tinycld/), NOT the workspace root:
// the canonical config relies on `.gitignore` to exclude generated/build
// artifacts (ios/, .expo, tinycld.config.ts, Podspecs, …), and the only
// .gitignore that lists them is tinycld/.gitignore. Once canonical is
// `root: false`, EVERY invocation under the workspace root (including
// `pnpm run lint` from tinycld/) resolves THIS config as the root and inherits
// its vcs settings — so this is where useIgnoreFile must be anchored. The bare
// workspace root has no .gitignore (and in a fresh bootstrap/CI assemble isn't
// even a git repo), so pointing biome there would make it error
// "couldn't find an ignore file".
function writeRootBiomeConfig() {
    const appDirName = path.basename(APP_DIR)
    const config = {
        $schema: 'https://biomejs.dev/schemas/2.4.16/schema.json',
        root: true,
        extends: [`./${appDirName}/biome.json`],
        vcs: { enabled: true, clientKind: 'git', useIgnoreFile: true, root: appDirName },
    }
    fs.writeFileSync(path.join(WS_ROOT, 'biome.json'), `${JSON.stringify(config, null, 4)}\n`)
}

// Slugs come from trusted manifests, but any code path that joins a slug into
// a path before rmSync gets this guard as defense-in-depth against a hostile
// or typo'd manifest escaping the intended base dir.
function assertSafeSlug(slug: string) {
    if (slug.includes('/') || slug.includes('..') || path.isAbsolute(slug)) {
        throw new Error(`[generate] invalid package slug '${slug}' — refusing to clean`)
    }
}

function cleanDir(dir: string) {
    // Safety: only ever rm -rf inside APP_DIR, and only under app/a/, app/p/,
    // or server/. Resolve first so a relative dir or one with '..' segments
    // can't smuggle the check, then require the resolved path to start with
    // APP_DIR and live under one of the allowed subtrees by path segment
    // (not substring — substring matches like .../app/personal/foo).
    const resolved = path.resolve(dir)
    const appPrefix = APP_DIR + path.sep
    if (!resolved.startsWith(appPrefix)) {
        throw new Error(`cleanDir refused: ${dir} is not under APP_DIR`)
    }
    const relSegments = path.relative(APP_DIR, resolved).split(path.sep)
    const allowedRoots = [['app', 'a'], ['app', 'p'], ['server']]
    const underAllowedRoot = allowedRoots.some(root =>
        root.every((seg, i) => relSegments[i] === seg)
    )
    if (!underAllowedRoot) {
        throw new Error(`cleanDir refused: ${dir} is not under app/a/, app/p/, or app/server/`)
    }
    if (fs.existsSync(dir)) fs.rmSync(dir, { recursive: true, force: true })
    fs.mkdirSync(dir, { recursive: true })
}

type Feature = { name: string; dir: string; manifest: PackageManifest }

// --- 3. routes: re-export each package's screens into app/a/[orgSlug]/<slug> -
// Do NOT cleanDir(ROUTES_BASE) — app-owned files live here (_layout.tsx,
// index.tsx, settings/**). Clean only each linked package's own slug dir.
// KNOWN TRADEOFF: a package unlinked since the last run leaves an orphan
// ROUTES_BASE/<old-slug>/ dir behind (the old full-wipe removed those). Fine
// while the linked set is stable; revisit (e.g. a generated-slugs manifest)
// if packages get unlinked frequently.
function emitFeatureRoutes(features: Feature[]) {
    fs.mkdirSync(ROUTES_BASE, { recursive: true })
    fs.mkdirSync(PUBLIC_ROUTES_BASE, { recursive: true })
    for (const f of features) {
        if (f.manifest.routes?.directory) emitOrgRoutes(f)
        if (f.manifest.publicRoutes?.directory) emitFeaturePublicRoutes(f)
    }
}

function emitOrgRoutes(f: Feature) {
    const slug = f.manifest.slug
    assertSafeSlug(slug)
    const slugDir = path.join(ROUTES_BASE, slug)
    if (fs.existsSync(slugDir)) fs.rmSync(slugDir, { recursive: true, force: true })
    const routesDir = resolveExportDir(f.dir, f.manifest.routes!.directory)
    if (!routesDir) {
        console.warn(
            `[generate] ${f.name}: no exports entry for './${f.manifest.routes!.directory}/*' — routes skipped`
        )
        return
    }
    emitRoutes({
        packageName: f.name,
        slug,
        packageDir: f.dir,
        routesDir,
        importSubpath: f.manifest.routes!.directory,
        routesBase: ROUTES_BASE,
    })
}

function emitFeaturePublicRoutes(f: Feature) {
    const slug = f.manifest.slug
    assertSafeSlug(slug)
    const slugDir = path.join(PUBLIC_ROUTES_BASE, slug)
    if (fs.existsSync(slugDir)) fs.rmSync(slugDir, { recursive: true, force: true })
    const pubDir = resolveExportDir(f.dir, f.manifest.publicRoutes!.directory)
    if (!pubDir) {
        console.warn(
            `[generate] ${f.name}: no exports entry for './${f.manifest.publicRoutes!.directory}/*' — public routes skipped`
        )
        return
    }
    emitPublicRoutes({
        packageName: f.name,
        slug,
        packageDir: f.dir,
        routesDir: pubDir,
        importSubpath: f.manifest.publicRoutes!.directory,
        publicRoutesBase: PUBLIC_ROUTES_BASE,
    })
}

// --- 4. package-help.ts (core + features) ------------------------------
function emitHelp(features: Feature[]) {
    const coreHelpDir = path.join(memberDir('@tinycld/core'), 'help')
    const helpSources: Feature[] = [
        // core help (core has a help/ dir but no manifest; include explicitly)
        ...(fs.existsSync(coreHelpDir)
            ? [
                  {
                      name: '@tinycld/core',
                      dir: memberDir('@tinycld/core'),
                      manifest: { help: { directory: 'help' }, slug: 'core' } as PackageManifest,
                  },
              ]
            : []),
        ...features,
    ]
    const helpGroups: HelpGroupInput[] = []
    for (const src of helpSources) {
        const group = readHelpGroup(src)
        if (group) helpGroups.push(group)
    }
    fs.writeFileSync(path.join(GENERATED_DIR, 'package-help.ts'), buildHelpSource(helpGroups))
}

function readHelpGroup(src: Feature): HelpGroupInput | null {
    if (!src.manifest.help?.directory) return null
    const helpDir = path.join(src.dir, src.manifest.help.directory)
    if (!fs.existsSync(helpDir)) return null
    const topics = fs
        .readdirSync(helpDir)
        .filter(f => f.endsWith('.md'))
        .map(file => ({
            topicId: file.replace(/\.md$/, ''),
            frontmatter: parseFrontmatter(fs.readFileSync(path.join(helpDir, file), 'utf8')),
        }))
    if (topics.length === 0) return null
    return { packageName: src.name, pkgSlug: src.manifest.slug, topics }
}

// --- 6. server: migration + hook symlinks ------------------------------
// The symlink merge flattens every package's pb-migrations/ into one directory,
// which is what PocketBase needs but loses each migration's owning package.
// Per-package version changes (apply/revert a single package's named migrations
// without touching others) need that attribution back, so as we link we record a
// migration-file → owning-slug map and emit it as pb_migrations_owner.json for
// the Go server to read at boot. Core migrations are owned by slug 'core'.
function symlinkServerArtifacts(features: Feature[]) {
    fs.mkdirSync(SERVER_DIR, { recursive: true })
    cleanDir(MIGRATIONS_DIR)
    cleanDir(HOOKS_DIR)
    const migrationOwners: Record<string, string> = {}
    // Tracks hook filenames across packages to reject cross-package collisions in
    // the flat HOOKS_DIR (not persisted — purely a guard).
    const hookOwners: Record<string, string> = {}
    // core migrations first (core has no manifest; include explicitly)
    linkDirContents(
        path.join(memberDir('@tinycld/core'), 'server', 'pb_migrations'),
        MIGRATIONS_DIR,
        'core',
        migrationOwners
    )
    for (const f of features) {
        if (f.manifest.migrations?.directory) {
            linkDirContents(
                path.join(f.dir, f.manifest.migrations.directory),
                MIGRATIONS_DIR,
                f.manifest.slug,
                migrationOwners
            )
        }
        if (f.manifest.hooks?.directory) {
            linkDirContents(
                path.join(f.dir, f.manifest.hooks.directory),
                HOOKS_DIR,
                f.manifest.slug,
                hookOwners,
                'hook'
            )
        }
    }
    fs.writeFileSync(
        path.join(SERVER_DIR, 'pb_migrations_owner.json'),
        `${JSON.stringify(migrationOwners, null, 2)}\n`
    )
}

// Symlink every regular file in `srcDir` into `destDir` (no-op if srcDir absent).
// When ownerSlug + owners are supplied, each linked filename is recorded as owned
// by ownerSlug. A filename collision across packages is a hard error: the flat
// migrations/hooks dirs require globally-unique filenames, and a silent overwrite
// would mis-attribute the file and let one package clobber another's (for hooks,
// silently dropping a package's hook). `kind` only labels the error message.
function linkDirContents(
    srcDir: string,
    destDir: string,
    ownerSlug?: string,
    owners?: Record<string, string>,
    kind: 'migration' | 'hook' = 'migration'
) {
    if (!fs.existsSync(srcDir)) return
    for (const file of fs.readdirSync(srcDir)) {
        const srcPath = path.join(srcDir, file)
        if (!fs.statSync(srcPath).isFile()) continue
        if (owners && ownerSlug) {
            if (owners[file] && owners[file] !== ownerSlug) {
                throw new Error(
                    `[generate] ${kind} filename collision: '${file}' is provided by both ` +
                        `'${owners[file]}' and '${ownerSlug}'. ${kind} filenames must be globally unique.`
                )
            }
            owners[file] = ownerSlug
        }
        replaceSymlink(srcPath, path.join(destDir, file))
    }
}

// buildCoreManifest synthesizes a minimal manifest for @tinycld/core (which has
// no manifest.ts) so it can be seeded into pkg_registry. Only the fields the
// registry seed + compatibility solver read are populated: name, slug, version,
// and any peerVersions core itself declares (read from core/package.json under a
// `tinycld.peerVersions` key, if present). nav is omitted so core never appears
// in the nav rail.
function buildCoreManifest(): PackageManifest {
    const corePkgJson = JSON.parse(
        fs.readFileSync(path.join(memberDir('@tinycld/core'), 'package.json'), 'utf8')
    )
    const peerVersions = corePkgJson.tinycld?.peerVersions as Record<string, string> | undefined
    return {
        name: '@tinycld/core',
        slug: 'core',
        version: corePkgJson.version ?? '',
        description: 'Shared core library',
        ...(peerVersions ? { peerVersions } : {}),
    }
}

// --- 7. server: Go wiring (package_extensions.go + go.work) ------------
function emitGoWiring(features: Feature[]) {
    const serverFeatures = features.filter(hasServerPackage)
    const serverPkgs: ServerPkg[] = serverFeatures.map(f => ({
        slug: f.manifest.slug,
        module: f.manifest.server!.module!,
        serverRelPath: path.relative(SERVER_DIR, path.join(f.dir, f.manifest.server!.package!)),
    }))
    fs.writeFileSync(
        path.join(SERVER_DIR, 'package_extensions.go'),
        buildPackageExtensionsGo(serverPkgs)
    )
    const coreServerDir = path.join(memberDir('@tinycld/core'), 'server')
    const coreServerRel = path.relative(SERVER_DIR, coreServerDir)
    const goWork = path.join(SERVER_DIR, 'go.work')
    // Always emit go.work, even with zero feature servers. The app server's
    // go.mod carries `replace tinycld.org/core => ../core/server`, so core is a
    // local-replace dependency on every assembly. Without a go.work the build
    // drops to single-module mode and verifies core's transitive deps against
    // app server/go.sum — which only records the app's own direct deps, not the
    // `/go.mod` hashes of deps reached through core (e.g. Masterminds/semver via
    // coreserver/pkg_compat.go), so it fails with "missing go.sum entry for
    // go.mod file". In workspace mode those hashes live in go.work.sum, which
    // `go mod download` regenerates. buildGoWork always lists `.` + core; an
    // empty serverPkgs just omits the feature `use` lines.
    fs.writeFileSync(goWork, buildGoWork(coreServerRel, serverPkgs))

    // Per-member go.work so each server module resolves tinycld.org/core when
    // built on its own (the app build above is unaffected — it runs from
    // app/server with its own go.work). core itself has nothing to wire.
    for (const f of serverFeatures) {
        if (f.manifest.slug === 'core') continue
        const memberServerDir = path.join(f.dir, f.manifest.server!.package!)
        const coreRelFromMember = path.relative(memberServerDir, coreServerDir)
        fs.writeFileSync(
            path.join(memberServerDir, 'go.work'),
            buildMemberGoWork(coreRelFromMember)
        )
    }
}

function hasServerPackage(f: Feature): boolean {
    if (!f.manifest.server?.package) return false
    if (!fs.existsSync(path.join(f.dir, f.manifest.server.package))) return false
    if (!f.manifest.server.module) {
        console.warn(
            `[generate] ${f.manifest.slug}: server.package declared but server.module is missing — Go wiring skipped`
        )
        return false
    }
    return true
}

async function main() {
    const featureNames = getPackages().filter(n => n !== '@tinycld/core')

    // Load each FEATURE manifest (core has none).
    const features: Feature[] = await Promise.all(
        featureNames.map(async name => {
            const dir = memberDir(name)
            const manifest = await loadManifest(dir)
            return { name, dir, manifest }
        })
    )

    // --- 0. package builds (e.g. text's webview-editor → editorHtml.ts) ----
    // Run any manifest.build scripts first so their outputs exist for the
    // config emit + the subsequent typecheck/bundle.
    const builds: BuildPkg[] = features
        .filter(f => f.manifest.build?.script)
        .map(f => ({ packageName: f.name, packageDir: f.dir, script: f.manifest.build!.script }))
    runPackageBuilds(WS_ROOT, builds)

    fs.mkdirSync(GENERATED_DIR, { recursive: true })

    // --- 1. tinycld.config.ts + tinycld.seeds.ts (at app root) -------------
    const configPkgs: ConfigPkg[] = features.map(f => manifestToConfigPkg(f.name, f.manifest))
    validateSidebarContributions(configPkgs)
    fs.writeFileSync(path.join(APP_DIR, 'tinycld.config.ts'), buildConfigSource(configPkgs))
    fs.writeFileSync(path.join(APP_DIR, 'tinycld.seeds.ts'), buildSeedsSource(configPkgs))

    // --- 1b. @tinycld/app-generated package manifest -----------------------
    // Makes lib/generated/ a real, name-resolvable package so `@tinycld/app-generated/*`
    // resolves by name (via the node_modules/@tinycld/app-generated symlink that
    // link-members.ts creates) — no consumer tsconfig `paths` entry required. The
    // 4-candidate exports cover files and directory-modules in both extensions.
    fs.writeFileSync(
        path.join(GENERATED_DIR, 'package.json'),
        `${JSON.stringify(
            {
                name: '@tinycld/app-generated',
                version: '0.0.0',
                private: true,
                exports: {
                    './*': ['./*.ts', './*.tsx', './*/index.ts', './*/index.tsx'],
                },
            },
            null,
            4
        )}\n`
    )

    // --- 2. @tinycld/app-generated/tinycld-config re-export shim ------------
    // core imports `@tinycld/app-generated/tinycld-config`; the app supplies it.
    // Use NAMED re-exports, not `export *`: tinycld.config.ts transitively
    // imports each package's collections/provider, which import
    // @tinycld/core/lib/pocketbase, which eagerly calls
    // buildPackageStores(tinycldConfig) at module-eval — a cycle. Under vitest's
    // ESM transform a wildcard re-export leaves `tinycldConfig` undefined while
    // the cycle resolves ("entries is not iterable"); a named re-export creates
    // a proper live binding that settles once the source finishes. (Metro
    // tolerates either, but the test loader does not.)
    fs.writeFileSync(
        path.join(GENERATED_DIR, 'tinycld-config.ts'),
        "// Auto-generated — re-export of app's tinycld.config.ts\nexport { tinycldConfig } from '../../tinycld.config'\nexport type { MergedPackageSchema } from '../../tinycld.config'\n"
    )

    emitFeatureRoutes(features)
    emitHelp(features)

    fs.writeFileSync(
        path.join(GENERATED_DIR, 'package-icons.ts'),
        buildPackageIconsSource(features.map(f => ({ name: f.name, manifest: f.manifest })))
    )

    // --- 5. uniwind-sources.css (core + features, real paths) --------------
    const uniwindSources: UniwindSource[] = [
        { packageName: '@tinycld/core', packageDir: fs.realpathSync(memberDir('@tinycld/core')) },
        ...features.map(f => ({ packageName: f.name, packageDir: fs.realpathSync(f.dir) })),
    ]
    fs.writeFileSync(
        path.join(GENERATED_DIR, 'uniwind-sources.css'),
        buildUniwindSources(uniwindSources)
    )

    symlinkServerArtifacts(features)
    emitGoWiring(features)
    // Seed a synthetic `core` manifest so @tinycld/core gets a pkg_registry row.
    // core has no manifest.ts and isn't a feature, but the compatibility solver
    // needs core's version resolvable so other packages' `@tinycld/core` peer
    // ranges can be satisfied (otherwise every such constraint is unsatisfiable).
    const coreManifest = buildCoreManifest()
    fs.writeFileSync(
        path.join(SERVER_DIR, 'bundled-packages.json'),
        buildBundledPackages([
            { manifest: coreManifest },
            ...features.map(f => ({ manifest: f.manifest })),
        ])
    )

    writeRootBiomeConfig()

    console.log(`Generated config for ${features.length} feature package(s).`)
}

main().catch(err => {
    console.error(err)
    process.exit(1)
})
