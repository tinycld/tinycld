//go:build unix

package listeners

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Set is an ordered collection of named listeners and files the supervisor
// hands to one child process. Order matters: it is the order ExtraFiles
// (and so fd numbers) and EnvFDNames are written in.
type Set struct {
	names []string
	files []*os.File
	addrs map[string]net.Addr
}

// AddListener adds l under name. It keeps l.File(), a dup of the listening
// socket; the dup is what a child's net.FileListener re-wraps, and what
// the server code that accepts on it ultimately closes, so the
// supervisor's own listener is never closed by a child's shutdown.
func (s *Set) AddListener(name string, l *net.TCPListener) error {
	f, err := l.File()
	if err != nil {
		return fmt.Errorf("listeners: dup %s listener: %w", name, err)
	}
	s.add(name, f)
	if s.addrs == nil {
		s.addrs = map[string]net.Addr{}
	}
	s.addrs[name] = l.Addr()
	return nil
}

// AddFile adds a plain, non-listener fd under name (e.g. the control
// socket end handed to a child).
func (s *Set) AddFile(name string, f *os.File) {
	s.add(name, f)
}

func (s *Set) add(name string, f *os.File) {
	s.names = append(s.names, name)
	s.files = append(s.files, f)
}

// Addr returns the bound address of the listener added under name, or nil
// if name was never added via AddListener.
func (s *Set) Addr(name string) net.Addr {
	return s.addrs[name]
}

// Apply sets cmd.ExtraFiles to this Set's files and appends EnvFDs/
// EnvFDNames to cmd.Env, in the same order, so the child's fd 3, 4, 5, …
// line up with names[0], names[1], names[2], ….
func (s *Set) Apply(cmd *exec.Cmd) {
	cmd.ExtraFiles = append(cmd.ExtraFiles, s.files...)
	cmd.Env = append(cmd.Env,
		EnvFDs+"="+strconv.Itoa(len(s.names)),
		EnvFDNames+"="+strings.Join(s.names, ":"),
	)
}
