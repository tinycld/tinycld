package backup

import "os"

// bootProbeEnv names the variable a supervisor sets on a process it starts ONLY
// to ask "does this build boot?" and then kills.
//
// Such a probe is a full server on the real data directory, so without this it
// performs the restore swap and the finalize that belong to the real boot. A
// kill landing between the swap writing its marker and the finalize removing it
// leaves the next boot — the real one — reading a swapped marker with no
// finished restore behind it, so it rolls the restore back. The operator's
// restore then silently did nothing.
//
// The name says "boot probe" and nothing about any particular supervisor: core
// does not know what starts it.
const bootProbeEnv = "TINYCLD_BOOT_PROBE"

// IsBootProbe reports whether this process was started only to check that the
// build boots. Such a process must leave every piece of restore state for the
// real boot to act on.
func IsBootProbe() bool { return os.Getenv(bootProbeEnv) == "1" }
