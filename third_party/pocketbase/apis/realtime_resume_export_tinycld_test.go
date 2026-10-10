package apis

import "time"

// SetRealtimeResumeLimits overrides the resume grace period and the queue
// limit for a test, and returns the function that restores them.
func SetRealtimeResumeLimits(grace time.Duration, maxMessages int) func() {
	oldGrace, oldMax := realtimeResumeGrace, realtimeResumeMaxMessages
	realtimeResumeGrace, realtimeResumeMaxMessages = grace, maxMessages

	return func() {
		realtimeResumeGrace, realtimeResumeMaxMessages = oldGrace, oldMax
	}
}
