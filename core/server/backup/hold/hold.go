// Package hold stops storage deletes while a backup reads the files its
// database snapshot refers to. The holder may be this process or another
// process on the same host, so the hold is a file in pb_data rather than
// memory.
//
// The hold is a lease: a crashed holder cannot block deletes for longer than
// Lease. A delete that arrives under a valid hold is journaled, and Drain
// runs the journal once no valid hold exists.
package hold

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	FileName    = "backup-hold"
	JournalName = "backup-hold.journal"
	drainName   = "backup-hold.journal.draining"
	Lease       = time.Hour
	RenewEvery  = 20 * time.Minute
)

var ErrHeld = errors.New("backup: another backup holds storage deletes")

type State struct {
	Holder  string    `json:"holder"`
	Expires time.Time `json:"expires"`
}

func (s State) Valid(now time.Time) bool { return now.Before(s.Expires) }

func Read(dataDir string) (State, bool, error) {
	raw, err := os.ReadFile(filepath.Join(dataDir, FileName))
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		// An unreadable hold holds nothing: treating it as valid would block
		// deletes forever with nothing to expire it.
		return State{}, false, nil
	}
	return st, true, nil
}

func Active(dataDir string, now time.Time) bool {
	st, ok, err := Read(dataDir)
	return err == nil && ok && st.Valid(now)
}

type Hold struct {
	dataDir, holder string
	now             func() time.Time
	stop            chan struct{}
	done            chan struct{}
	once            sync.Once
}

// Acquire takes the hold and renews it every RenewEvery until Release. An
// expired hold of another holder is taken over.
func Acquire(dataDir, holder string, now func() time.Time) (*Hold, error) {
	if now == nil {
		now = time.Now
	}
	path := filepath.Join(dataDir, FileName)
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			werr := writeState(f, State{Holder: holder, Expires: now().Add(Lease)})
			if werr != nil {
				_ = os.Remove(path)
				return nil, werr
			}
			h := &Hold{dataDir: dataDir, holder: holder, now: now, stop: make(chan struct{}), done: make(chan struct{})}
			go h.renew()
			return h, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		st, ok, rerr := Read(dataDir)
		if rerr != nil {
			return nil, rerr
		}
		if ok && st.Valid(now()) {
			return nil, fmt.Errorf("%w (holder %s)", ErrHeld, st.Holder)
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return nil, ErrHeld
}

func writeState(f *os.File, st State) error {
	raw, err := json.Marshal(st)
	if err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (h *Hold) renew() {
	defer close(h.done)
	t := time.NewTicker(RenewEvery)
	defer t.Stop()
	for {
		select {
		case <-h.stop:
			return
		case <-t.C:
			// Replace atomically so a reader never sees a half-written file.
			tmp := filepath.Join(h.dataDir, FileName+".tmp")
			f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
			if err != nil {
				continue
			}
			if writeState(f, State{Holder: h.holder, Expires: h.now().Add(Lease)}) == nil {
				_ = os.Rename(tmp, filepath.Join(h.dataDir, FileName))
			}
		}
	}
}

// Release stops renewal and removes the hold if it is still this holder's.
func (h *Hold) Release() error {
	var err error
	h.once.Do(func() {
		close(h.stop)
		<-h.done
		st, ok, rerr := Read(h.dataDir)
		if rerr != nil {
			err = rerr
			return
		}
		if ok && st.Holder == h.holder {
			if rerr := os.Remove(filepath.Join(h.dataDir, FileName)); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
				err = rerr
			}
		}
	})
	return err
}

func Journal(dataDir, key string) error {
	if strings.ContainsAny(key, "\n\r") {
		return fmt.Errorf("backup: storage key contains a line break")
	}
	f, err := os.OpenFile(filepath.Join(dataDir, JournalName), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(key + "\n"); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Drain deletes every journaled key when no valid hold exists. The journal is
// renamed before it is read, so deletes journaled under a hold that starts
// during the drain go to a new journal. A drain that stops on an error keeps
// the renamed file, and the next drain repeats it; del must treat a missing
// key as success.
func Drain(dataDir string, now time.Time, del func(key string) error) (int, error) {
	if Active(dataDir, now) {
		return 0, nil
	}
	draining := filepath.Join(dataDir, drainName)
	if _, err := os.Stat(draining); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(filepath.Join(dataDir, JournalName), draining); errors.Is(err, os.ErrNotExist) {
			return 0, nil
		} else if err != nil {
			return 0, err
		}
	}
	f, err := os.Open(draining)
	if err != nil {
		return 0, err
	}
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		key := sc.Text()
		if key == "" {
			continue
		}
		if err := del(key); err != nil {
			_ = f.Close()
			return n, err
		}
		n++
	}
	if err := sc.Err(); err != nil {
		_ = f.Close()
		return n, err
	}
	if err := f.Close(); err != nil {
		return n, err
	}
	return n, os.Remove(draining)
}

func RemoveStale(dataDir string, now time.Time) (State, bool, error) {
	st, ok, err := Read(dataDir)
	if err != nil || !ok || st.Valid(now) {
		return st, false, err
	}
	if err := os.Remove(filepath.Join(dataDir, FileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return st, false, err
	}
	return st, true, nil
}
