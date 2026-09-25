import { tinycldConfig } from '@tinycld/app-generated/tinycld-config'
import type { PackageSetupStep } from '@tinycld/core/lib/packages/config-types'
import { compareStepOrder } from '@tinycld/core/lib/setup/order'
import type { SetupStepEntry, SetupStepModule } from '@tinycld/core/lib/setup/types'
import { WORKSPACE_STEP_ID } from '@tinycld/core/lib/setup/wizard-logic'

type Loader = () => Promise<SetupStepModule>

// Core has no manifest, so its steps are listed here. Keys leave room on both
// sides for package steps (see docs/packages.md "setupSteps").
const CORE_STEPS: SetupStepEntry[] = [
    {
        id: WORKSPACE_STEP_ID,
        label: 'Workspace',
        order: 'a0',
        load: () => import('@tinycld/core/components/setup/wizard/steps/WorkspaceStep'),
    },
    {
        id: 'core:apps',
        label: 'Apps',
        order: 'a1',
        load: () => import('@tinycld/core/components/setup/wizard/steps/AppsStep'),
    },
    {
        id: 'core:email',
        label: 'Email sending',
        order: 'a2',
        load: () => import('@tinycld/core/components/setup/wizard/steps/EmailStep'),
    },
    {
        id: 'core:team',
        label: 'Your team',
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
            // The generator only emits modules that follow the step contract;
            // the registry normalizes the shape when it loads them.
            load: s.load as Loader,
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
