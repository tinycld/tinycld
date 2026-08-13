import { tinycldConfig } from '@tinycld/app-generated/tinycld-config'
import { CORE_AUTOMATION, CORE_PKG_SLUG } from '../automation/core-defs'
import { qualifyRef } from '../automation/helpers'
import type { ActionDef, AutomationDefinitions, TriggerDef } from '../automation/types'

export interface CatalogTrigger {
    pkgSlug: string
    pkgName: string
    def: TriggerDef
}
export interface CatalogAction {
    pkgSlug: string
    pkgName: string
    def: ActionDef
}
export interface AutomationPackageGroup {
    pkgSlug: string
    pkgName: string
    triggers: TriggerDef[]
    actions: ActionDef[]
}
export interface AutomationCatalog {
    triggers: Record<string, CatalogTrigger>
    actions: Record<string, CatalogAction>
    byPackage: AutomationPackageGroup[]
}

type AutomationEntryLike = {
    manifest: { name: string; slug: string }
    automation?: AutomationDefinitions
}

/** Ref-keyed trigger/action catalogs; core built-ins always present, first. */
export function deriveAutomation(entries: readonly AutomationEntryLike[]): AutomationCatalog {
    const catalog: AutomationCatalog = { triggers: {}, actions: {}, byPackage: [] }
    const sources: { pkgSlug: string; pkgName: string; defs: AutomationDefinitions }[] = [
        { pkgSlug: CORE_PKG_SLUG, pkgName: 'Core', defs: CORE_AUTOMATION },
    ]
    for (const e of entries) {
        if (!e.automation) continue
        sources.push({ pkgSlug: e.manifest.slug, pkgName: e.manifest.name, defs: e.automation })
    }
    for (const { pkgSlug, pkgName, defs } of sources) {
        const triggers = defs.triggers ?? []
        const actions = defs.actions ?? []
        for (const def of triggers) {
            catalog.triggers[qualifyRef(pkgSlug, def.id)] = { pkgSlug, pkgName, def }
        }
        for (const def of actions) {
            catalog.actions[qualifyRef(pkgSlug, def.id)] = { pkgSlug, pkgName, def }
        }
        catalog.byPackage.push({ pkgSlug, pkgName, triggers, actions })
    }
    return catalog
}

export const packageAutomation: AutomationCatalog = deriveAutomation(tinycldConfig)
