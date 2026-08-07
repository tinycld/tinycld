import { spawnSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { memberDir } from './paths'
import { assertSafeImportField } from './validate-generated-field'

// Emit lib/generated/<slug>-api.ts from each package's Go payload package —
// the exported HTTP request/response structs it declares via the manifest
// `payloads: { package }` block. The Go structs are the single source of
// truth for API payload shapes; TS hooks and (later) the CLI both consume
// them, so a field edit lands everywhere from one place.
//
// Unlike export-types.ts this needs no PocketBase boot, no tmpdir pb_data,
// and no migration replay — the export-payload-types binary parses the
// payload package's source as text (go/ast) and never imports it, keeping
// the toolchain requirement identical (just `go` on PATH; Docker's
// web-builder stage supplies a prebuilt binary via
// TINYCLD_EXPORT_PAYLOADS_BIN). It runs as a generate.ts step (not a
// packages:generate sibling) so the dev loop regenerates payload types too.

export interface PayloadFeatureInput {
    name: string // e.g. '@tinycld/mail' — used in error messages
    dir: string // member root on disk
    manifest: { slug: string; payloads?: { package: string } }
}

export interface PayloadEmit {
    slug: string
    srcDir: string
    outFile: string
}

// Plan one emit per feature that declares a payloads block. The package dir is
// manifest-derived and joined into paths, so it gets the import-field allowlist
// plus an explicit traversal rejection (defense-in-depth, mirroring
// assertSafeSlug in generate.ts).
export function planPayloadEmits(
    features: PayloadFeatureInput[],
    generatedDir: string
): PayloadEmit[] {
    const emits: PayloadEmit[] = []
    for (const f of features) {
        const pkg = f.manifest.payloads?.package
        if (!pkg) continue
        assertSafeImportField(`payloads.package (${f.name})`, pkg)
        if (pkg.includes('..') || path.isAbsolute(pkg)) {
            throw new Error(
                `[gen-payload-types] ${f.name}: payloads.package '${pkg}' must be a relative path inside the member`
            )
        }
        const slug = f.manifest.slug
        if (slug.includes('/') || slug.includes('..') || path.isAbsolute(slug)) {
            throw new Error(`[gen-payload-types] ${f.name}: invalid slug '${slug}'`)
        }
        emits.push({
            slug,
            srcDir: path.join(f.dir, pkg),
            outFile: path.join(generatedDir, `${slug}-api.ts`),
        })
    }
    return emits
}

// A `<slug>-api.ts` whose slug is no longer a present feature is stale output
// from a removed member — return it for deletion so the generated dir mirrors
// the installed set exactly (same rationale as pruneOrphanRouteDirs).
export function orphanPayloadFiles(generatedDir: string, presentSlugs: Set<string>): string[] {
    let entries: string[]
    try {
        entries = fs.readdirSync(generatedDir)
    } catch {
        return []
    }
    const orphans: string[] = []
    for (const entry of entries) {
        const match = /^(.+)-api\.ts$/.exec(entry)
        if (match && !presentSlugs.has(match[1])) {
            orphans.push(path.join(generatedDir, entry))
        }
    }
    return orphans
}

export function runPayloadEmits(emits: PayloadEmit[]): void {
    if (emits.length === 0) return
    const cmdDir = path.resolve(memberDir('@tinycld/core'), 'server', 'cmd', 'export-payload-types')
    const prebuilt = process.env.TINYCLD_EXPORT_PAYLOADS_BIN
    for (const emit of emits) {
        const args = ['--src', emit.srcDir, '--out', emit.outFile]
        const result = prebuilt
            ? spawnSync(prebuilt, args, { stdio: 'inherit' })
            : spawnSync('go', ['run', '.', ...args], { cwd: cmdDir, stdio: 'inherit' })
        if (result.status !== 0) {
            throw new Error(
                `[gen-payload-types] payload type generation failed for '${emit.slug}' (status ${result.status ?? 'unknown'})`
            )
        }
    }
}
