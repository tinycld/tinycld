import { describe, expect, it } from 'vitest'
import { shouldAdoptRunningJob } from '../adopt-running-job'

// The Packages screen adopts an already-running job so the panel shows it
// rather than looking idle mid-rebuild. That adoption runs during render, so
// it has to respect a job the admin closed — otherwise Close clears the
// panel's job and the next render immediately re-adopts the same one, and the
// button reads as broken.
const never = () => false

describe('shouldAdoptRunningJob', () => {
    it('adopts a running job when the panel is idle', () => {
        expect(
            shouldAdoptRunningJob({
                runningJobId: 'job_1',
                hasPanelJob: false,
                hasApplyJob: false,
                isDismissed: never,
            })
        ).toBe(true)
    })

    it('does not adopt a job the admin already closed', () => {
        expect(
            shouldAdoptRunningJob({
                runningJobId: 'job_1',
                hasPanelJob: false,
                hasApplyJob: false,
                isDismissed: id => id === 'job_1',
            })
        ).toBe(false)
    })

    it('still adopts a different job after one was dismissed', () => {
        expect(
            shouldAdoptRunningJob({
                runningJobId: 'job_2',
                hasPanelJob: false,
                hasApplyJob: false,
                isDismissed: id => id === 'job_1',
            })
        ).toBe(true)
    })

    it('does nothing when there is no running job', () => {
        expect(
            shouldAdoptRunningJob({
                runningJobId: undefined,
                hasPanelJob: false,
                hasApplyJob: false,
                isDismissed: never,
            })
        ).toBe(false)
    })

    // The panel renders in one place for both sources; adopting on top of a
    // job the panel is already tracking would swap it mid-run.
    it('leaves a job the panel is already showing alone', () => {
        expect(
            shouldAdoptRunningJob({
                runningJobId: 'job_1',
                hasPanelJob: true,
                hasApplyJob: false,
                isDismissed: never,
            })
        ).toBe(false)
    })

    it('yields to an apply job, which owns the panel itself', () => {
        expect(
            shouldAdoptRunningJob({
                runningJobId: 'job_1',
                hasPanelJob: false,
                hasApplyJob: true,
                isDismissed: never,
            })
        ).toBe(false)
    })
})
