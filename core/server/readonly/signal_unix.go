//go:build unix

package readonly

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var watchOnce sync.Once

// watchSignal makes SIGUSR2 enter read-only mode. Without a handler, SIGUSR2
// would terminate the process, so it is installed once, at registration.
func watchSignal() {
	watchOnce.Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGUSR2)
		go func() {
			for range ch {
				Enter()
			}
		}()
	})
}
