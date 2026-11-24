import {
    packageSidebarContributions,
    type SidebarContributionEntry,
} from '@tinycld/core/lib/packages/derive-components'
import { declaresTerms, useTermsOpen } from '@tinycld/core/lib/packages/use-package-terms'
import { type ComponentType, Suspense } from 'react'

type SlotProps = Record<string, unknown>

interface SidebarSlotProps {
    target: string
    slot: string
    // Passed to every contributed component. A slot documents its own props
    // contract (for example GroupMemberRemovalNoticeProps).
    props?: SlotProps
}

/** One contribution, loaded on first show. A host that lays contributions out itself renders each through this. */
export function SlotContribution({
    entry,
    props,
}: {
    entry: SidebarContributionEntry
    props?: SlotProps
}) {
    // The one place props meet a contribution. The registry erases prop
    // types, because each slot documents its own props contract and the
    // generated map holds components of many prop shapes. The host's props
    // are trusted to match that contract here.
    const Component = entry.Component as ComponentType<SlotProps>
    return (
        <Suspense fallback={null}>
            <Component {...props} />
        </Suspense>
    )
}

export function SidebarSlot({ target, slot, props }: SidebarSlotProps) {
    const entries = packageSidebarContributions[target]?.[slot]
    if (!entries || entries.length === 0) return null
    // Only a slot that holds a contribution of a package with terms asks for
    // the terms state; every other slot reads no store.
    if (entries.some(entry => declaresTerms(entry.contributorSlug))) {
        return <GatedContributions entries={entries} props={props} />
    }
    return <Contributions entries={entries} props={props} />
}

interface ContributionsProps {
    entries: readonly SidebarContributionEntry[]
    props?: SlotProps
}

function Contributions({ entries, props }: ContributionsProps) {
    return entries.map(entry => (
        <SlotContribution key={entry.contributorSlug} entry={entry} props={props} />
    ))
}

/** Leaves out the contributions of a package whose terms the owner has not accepted. */
function GatedContributions({ entries, props }: ContributionsProps) {
    const open = useTermsOpen(entries, contributorOf)
    return <Contributions entries={open} props={props} />
}

const contributorOf = (entry: SidebarContributionEntry) => entry.contributorSlug
