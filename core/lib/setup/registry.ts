import { tinycldConfig } from '@tinycld/app-generated/tinycld-config'
import type { PackageSetupStep } from '@tinycld/core/lib/packages/config-types'
import { compareStepOrder } from '@tinycld/core/lib/setup/order'
import { CORE_STEP_IDS } from '@tinycld/core/lib/setup/step-ids'
import type { SetupStepEntry } from '@tinycld/core/lib/setup/types'

// Core has no manifest, so its steps are listed here. Keys leave room on both
// sides for package steps (see docs/packages.md "setupSteps").
const CORE_STEPS: SetupStepEntry[] = [
    {
        id: CORE_STEP_IDS.workspace,
        label: 'Organization',
        order: 'a0',
        load: () => import('@tinycld/core/components/setup/wizard/steps/WorkspaceStep'),
    },
    {
        id: CORE_STEP_IDS.apps,
        label: 'Apps',
        order: 'a1',
        load: () => import('@tinycld/core/components/setup/wizard/steps/AppsStep'),
    },
    {
        id: CORE_STEP_IDS.email,
        label: 'Sending',
        order: 'a2',
        load: () => import('@tinycld/core/components/setup/wizard/steps/EmailStep'),
    },
    {
        id: CORE_STEP_IDS.team,
        label: 'Team',
        order: 'a3',
        load: () => import('@tinycld/core/components/setup/wizard/steps/TeamStep'),
    },
]

export function buildSetupStepEntries(
    core: SetupStepEntry[],
    pkgs: readonly { manifest: { slug: string }; setupSteps?: PackageSetupStep[] }[]
): SetupStepEntry[] {
    const fromPackages = pkgs.flatMap(p =>
        (p.setupSteps ?? []).map(s => ({
            id: `${p.manifest.slug}:${s.id}`,
            label: s.label,
            order: s.order,
            load: s.load,
        }))
    )
    return [...core, ...fromPackages].sort(compareStepOrder)
}

export const setupStepEntries = buildSetupStepEntries(CORE_STEPS, tinycldConfig)

// `:` is legal in a path but reads as a scheme separator to some routers and
// link parsers; slugs and step ids never contain `.`.
export function stepIdToParam(id: string): string {
    return id.replace(':', '.')
}

export function paramToStepId(param: string): string {
    return param.replace('.', ':')
}
