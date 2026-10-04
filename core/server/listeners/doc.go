// Package listeners passes already-bound TCP listeners from a supervisor
// process to a server child as named, inherited file descriptors, and lets
// the child look them up by name.
//
// The supervisor side builds a Set, adds listeners and plain files to it,
// then calls Apply before starting the child process:
//
//	set := &listeners.Set{}
//	set.AddListener("acme-secure", tcpListener)
//	set.Apply(cmd) // sets cmd.ExtraFiles, appends env to cmd.Env
//
// The child side looks a listener up by the same name:
//
//	l, ok := listeners.Inherited("acme-secure")
//	if !ok {
//	    l, _ = net.Listen("tcp", addr) // no supervisor: bind it directly
//	}
//
// Two environment variables carry the handoff, prefixed so a systemd
// LISTEN_FDS meant for something else is never mistaken for ours:
//
//	TINYCLD_LISTEN_FDS     the count of inherited fds, starting at fd 3
//	TINYCLD_LISTEN_FDNAMES colon-separated names, one per fd, in fd order
package listeners
