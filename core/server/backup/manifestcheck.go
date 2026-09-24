package backup

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/backup/format"
)

var ErrManifestMismatch = errors.New("backup: archive does not match this binary's package set")

// Diff is what a restore would have to change about this deployment to run the
// archive's package set. A deployment that can rebuild itself acts on it; one
// that cannot refuses, because restoring a database whose rows belong to
// packages this binary does not carry leaves data no screen can reach.
type Diff struct {
	Missing      []string             `json:"missing"`      // in the archive, not in this binary
	Extra        []string             `json:"extra"`        // in this binary, not in the archive
	VersionDelta map[string][2]string `json:"versionDelta"` // slug → [archive, binary]
}

func (d Diff) Empty() bool {
	return len(d.Missing) == 0 && len(d.Extra) == 0 && len(d.VersionDelta) == 0
}

type MismatchError struct{ Diff Diff }

func (e *MismatchError) Error() string {
	var parts []string
	if len(e.Diff.Missing) > 0 {
		parts = append(parts, "missing: "+strings.Join(e.Diff.Missing, ", "))
	}
	if len(e.Diff.Extra) > 0 {
		parts = append(parts, "extra: "+strings.Join(e.Diff.Extra, ", "))
	}
	for slug, v := range e.Diff.VersionDelta {
		parts = append(parts, fmt.Sprintf("%s: archive %s, binary %s", slug, v[0], v[1]))
	}
	// Sorted so the same difference always reads the same way: the message goes
	// in the ledger row's error column, where two spellings of one problem look
	// like two problems.
	sort.Strings(parts)
	return ErrManifestMismatch.Error() + " (" + strings.Join(parts, "; ") + ")"
}

func (e *MismatchError) Is(target error) bool { return target == ErrManifestMismatch }

func compareEmbedded(m format.Manifest, installed map[string]string) Diff {
	d := Diff{VersionDelta: map[string][2]string{}}
	for slug, v := range m.Packages {
		got, ok := installed[slug]
		if !ok {
			d.Missing = append(d.Missing, slug)
			continue
		}
		if got != v {
			d.VersionDelta[slug] = [2]string{v, got}
		}
	}
	for slug := range installed {
		if _, ok := m.Packages[slug]; !ok {
			d.Extra = append(d.Extra, slug)
		}
	}
	sort.Strings(d.Missing)
	sort.Strings(d.Extra)
	return d
}

// installedPackages is this binary's package set as the registry records it.
// core is left out: its version is a manifest field of its own, so counting it
// here would report a mismatch on every core upgrade.
func installedPackages(app core.App) (map[string]string, error) {
	regs, err := app.FindRecordsByFilter("pkg_registry", "status = 'installed' || status = 'bundled'", "slug", 0, 0)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, r := range regs {
		if slug := r.GetString("slug"); slug != "core" {
			out[slug] = r.GetString("version")
		}
	}
	return out, nil
}
