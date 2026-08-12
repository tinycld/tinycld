import * as fs from 'node:fs'
import * as path from 'node:path'
import { pathToFileURL } from 'node:url'
import { CORE_AUTOMATION, CORE_PKG_SLUG } from '../core/lib/automation/core-defs'
import { validateDefinitions } from '../core/lib/automation/schemas'
import type { ActionDef, AutomationDefinitions, TriggerDef } from '../core/lib/automation/types'
import { SERVER_DIR } from './paths'

export interface MergedAutomation {
    packages: { slug: string; triggers: TriggerDef[]; actions: ActionDef[] }[]
}

// Resolve an exports-map subpath to a file and import it, the same way tsx
// lets loadManifest import member TS directly. We read package.json ourselves
// (rather than createRequire) so the error names the missing entry precisely.
export async function loadAutomationDefs(
    packageDir: string,
    packageName: string,
    subpath: string
): Promise<AutomationDefinitions> {
    const pkgJsonPath = path.join(packageDir, 'package.json')
    const exportsMap = (JSON.parse(fs.readFileSync(pkgJsonPath, 'utf8')).exports ?? {}) as Record<
        string,
        string
    >
    const rel = exportsMap[`./${subpath}`]
    if (!rel) {
        throw new Error(
            `[generate] ${packageName}: manifest declares automation definitions '${subpath}' but package.json exports has no './${subpath}' entry`
        )
    }
    const mod = await import(pathToFileURL(path.join(packageDir, rel)).href)
    return mod.default as AutomationDefinitions
}

export function mergeAutomationDefs(
    features: { slug: string; defs: AutomationDefinitions }[]
): MergedAutomation {
    const errors = [
        ...validateDefinitions(CORE_PKG_SLUG, CORE_AUTOMATION, { allowSynthetic: true }),
        ...features.flatMap(f => validateDefinitions(f.slug, f.defs)),
    ]
    if (errors.length > 0) {
        throw new Error(`[generate] invalid automation definitions:\n  ${errors.join('\n  ')}`)
    }
    const sorted = [...features].sort((a, b) => a.slug.localeCompare(b.slug))
    return {
        packages: [
            {
                slug: CORE_PKG_SLUG,
                triggers: CORE_AUTOMATION.triggers ?? [],
                actions: CORE_AUTOMATION.actions ?? [],
            },
            ...sorted.map(f => ({
                slug: f.slug,
                triggers: f.defs.triggers ?? [],
                actions: f.defs.actions ?? [],
            })),
        ],
    }
}

// Materialized for the Go engine (Phase 2 input) — same idiom as the caldav
// manifest block: TS is the authoring format, the server consumes JSON.
export function emitAutomationDefs(merged: MergedAutomation): void {
    fs.writeFileSync(
        path.join(SERVER_DIR, 'automation_defs.json'),
        `${JSON.stringify(merged, null, 4)}\n`
    )
}
