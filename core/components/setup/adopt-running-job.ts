/**
 * Whether the Packages screen should adopt a job that is already running — one
 * started by another admin, or by this one before a reload — so the progress
 * panel shows it instead of leaving the screen looking idle mid-rebuild.
 *
 * `isDismissed` is what makes the panel's Close button work. Adoption runs
 * during render, so without it closing the panel (which clears the panel's own
 * job) is undone on the very next render: the same running job is re-adopted
 * and the panel reopens, making the button look dead.
 *
 * Lives in its own module so it can be unit-tested without pulling in the
 * screen's component graph.
 */
export function shouldAdoptRunningJob({
    runningJobId,
    hasPanelJob,
    hasApplyJob,
    isDismissed,
}: {
    runningJobId: string | undefined
    hasPanelJob: boolean
    hasApplyJob: boolean
    isDismissed: (jobId: string) => boolean
}): boolean {
    if (!runningJobId) return false
    if (hasPanelJob || hasApplyJob) return false
    return !isDismissed(runningJobId)
}
