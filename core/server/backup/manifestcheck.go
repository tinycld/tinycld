package backup

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"filippo.io/age"
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
	// No sort: the result is a map, so ordering the query would cost a sort
	// nothing reads.
	regs, err := app.FindRecordsByFilter("pkg_registry", "status = 'installed' || status = 'bundled'", "", 0, 0)
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

// CheckManifest reads only an archive's manifest and answers phase 2's question
// — can this deployment run that package set? — with NO side effects: nothing is
// staged, no ledger row is written, and the interlock is not claimed.
//
// It exists for the upload branch of the restore API, which must decide the
// 409-on-mismatch BEFORE it answers 202 and hands the archive to an asynchronous
// restore. runRestore repeats the same comparison; that duplication is deliberate
// rather than shared state, because a URL source cannot be re-read and must
// still be checked once the transfer is under way.
//
// A deployment with a rebuilder registered can become whatever the archive names,
// so it has nothing to refuse and the answer is nil.
func CheckManifest(app core.App, r io.Reader, identity age.Identity) (format.Manifest, error) {
	reader, err := format.NewReader(r, identity)
	if err != nil {
		return format.Manifest{}, err
	}
	defer reader.Close()
	m, err := reader.ReadManifest()
	if err != nil {
		return format.Manifest{}, err
	}
	if HasRebuilder() {
		return m, nil
	}
	installed, err := installedPackages(app)
	if err != nil {
		return m, err
	}
	if d := compareEmbedded(m, installed); !d.Empty() {
		return m, &MismatchError{Diff: d}
	}
	return m, nil
}
