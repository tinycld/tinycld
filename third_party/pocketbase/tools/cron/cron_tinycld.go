package cron

// SetSkip installs fn, which the scheduler asks once for each job that is due
// on a tick. When fn returns true, the job is not started on that tick; it
// runs again at its next due time. A nil fn removes the check.
//
// The fork adds this so a process can hold back every scheduled job for a
// while (for example while it refuses writes) without wrapping each job at
// the place that registers it, which the scheduler's own jobs and those added
// from JS hooks would escape.
func (c *Cron) SetSkip(fn func(jobId string) bool) {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.skip = fn
}

// skipped is called by runDue with the read lock held.
func (c *Cron) skipped(j *Job) bool {
	return c.skip != nil && c.skip(j.id)
}
