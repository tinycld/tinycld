import { create } from '@tinycld/core/lib/store'
import { compareVersions } from './version-compare'

// Staging state for the merged Packages screen: which packages have a pending
// version change the operator has selected but not yet applied. This is UI state
// (transient intent) — server truth (the available versions) and the mutations
// (compat-check, apply) live in use-package-versions. Keeping the staged set in
// a store decouples it from the row components and from the data hook, so a row's
// version select, the apply footer, and the "is this row staged?" lock can all
// read/write the same selection without prop-drilling.
//
// Not persisted: a half-staged version change shouldn't survive a reload (the
// available versions are re-fetched fresh each visit, and applying is a
// deliberate, in-session action).
interface VersionStagingState {
    // slug → target version (the raw discovered version string / git tag). A slug
    // is absent when its target equals current (i.e. nothing staged for it).
    targets: Record<string, string>
    // Set a slug's target. Passing the current version (or empty) clears it —
    // callers pass `currentVersion` so the store can drop no-op selections without
    // needing the full version list.
    setTarget: (slug: string, version: string, currentVersion: string) => void
    // Replace the whole staged set at once (used by "stage all updates").
    setAll: (targets: Record<string, string>) => void
    clear: () => void
}

export const useVersionStagingStore = create<VersionStagingState>()(set => ({
    targets: {},
    setTarget: (slug, version, currentVersion) =>
        set(s => {
            const next = { ...s.targets }
            // Compare semver-aware, not by string ===: option values are raw git
            // tags (`v1.0.0`) while currentVersion is the bare registry version
            // (`1.0.0`), so a raw compare would store the explicit `(current)`
            // selection as a no-op "stage" — making the apply footer count a change
            // the row's (semver-aware) flag/lock denies. compareVersions tolerates
            // the `v` prefix; === 0 means same version → clear the stage.
            if (!version || compareVersions(version, currentVersion) === 0) {
                delete next[slug]
            } else {
                next[slug] = version
            }
            return { targets: next }
        }),
    setAll: targets => set({ targets }),
    clear: () => set({ targets: {} }),
}))
