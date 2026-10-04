package cron

import (
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestCronSkipHoldsBackDueJobs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New()

		var ran atomic.Int32
		c.MustAdd("due", "* * * * *", func() { ran.Add(1) })

		var asked []string
		skip := true
		c.SetSkip(func(jobId string) bool {
			asked = append(asked, jobId)
			return skip
		})

		c.runDue(time.Now())
		synctest.Wait()
		if got := ran.Load(); got != 0 {
			t.Fatalf("a skipped job ran %d times", got)
		}
		if len(asked) != 1 || asked[0] != "due" {
			t.Fatalf("skip was asked for %v, want [due]", asked)
		}

		skip = false
		c.runDue(time.Now())
		synctest.Wait()
		if got := ran.Load(); got != 1 {
			t.Fatalf("the job ran %d times after the skip was lifted, want 1", got)
		}
	})
}

// A job that is not due is not offered to the skip function: the function
// sees only the ticks it can hold back.
func TestCronSkipAskedOnlyForDueJobs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New()
		c.MustAdd("never", "0 0 1 1 *", func() {})

		asked := 0
		c.SetSkip(func(string) bool { asked++; return false })

		c.runDue(time.Date(2026, 6, 15, 12, 30, 0, 0, time.UTC))
		synctest.Wait()
		if asked != 0 {
			t.Fatalf("skip was asked %d times for a job that is not due", asked)
		}
	})
}

func TestCronWithoutSkipRunsDueJobs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New()
		var ran atomic.Int32
		c.MustAdd("due", "* * * * *", func() { ran.Add(1) })

		c.runDue(time.Now())
		synctest.Wait()
		if got := ran.Load(); got != 1 {
			t.Fatalf("the job ran %d times, want 1", got)
		}
	})
}
