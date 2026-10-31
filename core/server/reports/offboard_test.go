package reports

import (
	"errors"
	"testing"

	"tinycld.org/core/offboard"
)

func (s *savedZoo) ownerOf(t *testing.T, key string) string {
	t.Helper()
	rec, err := s.app.FindRecordById(reportsCollection, s.id(key))
	if err != nil {
		return ""
	}
	return rec.GetString("owner")
}

func (s *savedZoo) shareExists(key string) bool {
	_, err := s.app.FindRecordById(sharesCollection, s.id(key))
	return err == nil
}

func (s *savedZoo) sharesOf(t *testing.T, report string) int {
	t.Helper()
	rows, err := s.app.FindRecordsByFilter(sharesCollection, "report = {:r}", "", 0, 0, map[string]any{"r": s.id(report)})
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

// Reassign: every report of the leaver goes to the successor, who loses the
// direct share ownership now covers; other shares stay. The leaver's
// shares on other people's reports go.
func TestOffboardReassignMovesEveryReport(t *testing.T) {
	s := newSavedZoo(t)
	s.share(t, "R2.ann", "R2", "ann", "", "viewer")
	if _, err := offboard.OffboardUser(s.app, s.id("ann"), offboard.Plan{Mode: offboard.ModeReassign, SuccessorUserID: s.id("ed")}, s.id("ad")); err != nil {
		t.Fatal(err)
	}
	if s.ownerOf(t, "R") != s.id("ed") || s.ownerOf(t, "P") != s.id("ed") {
		t.Fatalf("owners: R %s, P %s; want ed", s.ownerOf(t, "R"), s.ownerOf(t, "P"))
	}
	if s.shareExists("R.ed") || !s.shareExists("R.vi") || !s.shareExists("R.keepers") || s.shareExists("R2.ann") {
		t.Fatalf("shares: R.ed %v (want gone), R.vi %v, R.keepers %v, R2.ann %v (want gone)",
			s.shareExists("R.ed"), s.shareExists("R.vi"), s.shareExists("R.keepers"), s.shareExists("R2.ann"))
	}
}

// Delete my data: an unpublished report goes with every share row; a
// published one goes to the admin who ran the offboard.
func TestOffboardDeleteKeepsPublishedReports(t *testing.T) {
	s := newSavedZoo(t)
	if _, err := offboard.OffboardUser(s.app, s.id("ann"), offboard.Plan{Mode: offboard.ModeDeleteMyData}, s.id("ad")); err != nil {
		t.Fatal(err)
	}
	if s.ownerOf(t, "R") != "" || s.sharesOf(t, "R") != 0 {
		t.Fatalf("R: owner %q, %d shares; want deleted with its shares", s.ownerOf(t, "R"), s.sharesOf(t, "R"))
	}
	if s.ownerOf(t, "P") != s.id("ad") {
		t.Fatalf("P owner %s, want the acting admin", s.ownerOf(t, "P"))
	}
}

// Keep (a self-delete): an unpublished report stays with the anonymized
// account; a published one goes to the first admin.
func TestOffboardKeepMovesOnlyPublishedReports(t *testing.T) {
	s := newSavedZoo(t)
	owner := s.person(t, "own", "owner")
	if _, err := offboard.OffboardUser(s.app, s.id("ann"), offboard.Plan{Mode: offboard.ModeKeep}, s.id("ann")); err != nil {
		t.Fatal(err)
	}
	if s.ownerOf(t, "R") != s.id("ann") || s.sharesOf(t, "R") != 4 {
		t.Fatalf("R: owner %s, %d shares; want ann's, all 4 kept", s.ownerOf(t, "R"), s.sharesOf(t, "R"))
	}
	if s.ownerOf(t, "P") != owner.Id {
		t.Fatalf("P owner %s, want the first admin", s.ownerOf(t, "P"))
	}
}

// With nobody to take a published report, the offboard is refused and
// nothing changes.
func TestOffboardWithNoHeirIsRefused(t *testing.T) {
	s := newSavedZoo(t)
	s.recs["ad"].Set("role", "member")
	if err := s.app.Save(s.recs["ad"]); err != nil {
		t.Fatal(err)
	}
	_, err := offboard.OffboardUser(s.app, s.id("ann"), offboard.Plan{Mode: offboard.ModeDeleteMyData}, s.id("ann"))
	if !errors.Is(err, offboard.ErrInvalidPlan) {
		t.Fatalf("err = %v, want ErrInvalidPlan", err)
	}
	if s.ownerOf(t, "R") != s.id("ann") || s.ownerOf(t, "P") != s.id("ann") {
		t.Fatal("a refused offboard changed reports")
	}
	ann, _ := s.app.FindRecordById("users", s.id("ann"))
	if ann.Email() != "ann@zoo.test" {
		t.Fatal("a refused offboard anonymized the user")
	}
}

// A group member who leaves loses the derived row; the grant stays.
func TestOffboardRemovesAMembersDerivedRow(t *testing.T) {
	s := newSavedZoo(t)
	if _, err := offboard.OffboardUser(s.app, s.id("gm"), offboard.Plan{Mode: offboard.ModeKeep}, s.id("ad")); err != nil {
		t.Fatal(err)
	}
	if !s.shareExists("R.keepers") || s.sharesOf(t, "R") != 3 {
		t.Fatalf("R has %d shares, want 3 (ed, vi, the grant)", s.sharesOf(t, "R"))
	}
}

// A shipped report is never deleted, even unpublished: the next start would
// make it again published. It goes to the heir and stays unpublished.
func TestOffboardMovesAnUnpublishedShippedReport(t *testing.T) {
	s := newSavedZoo(t)
	row := s.saved(t, "S", "ann", "S", savedSpec, false)
	row.Set("shipped_key", "zoo.feed_costs")
	if err := s.app.Save(row); err != nil {
		t.Fatal(err)
	}
	if _, err := offboard.OffboardUser(s.app, s.id("ann"), offboard.Plan{Mode: offboard.ModeDeleteMyData}, s.id("ad")); err != nil {
		t.Fatal(err)
	}
	got, err := s.app.FindRecordById(reportsCollection, s.id("S"))
	if err != nil {
		t.Fatalf("the shipped report was deleted: %v", err)
	}
	if got.GetString("owner") != s.id("ad") || got.GetBool("published") {
		t.Fatalf("owner %s, published %v; want ad, unpublished", got.GetString("owner"), got.GetBool("published"))
	}
}

// An app without the reports migration owns no reports: the handler does nothing.
func TestOffboardWithoutReportsCollectionDoesNothing(t *testing.T) {
	s := newSavedZoo(t)
	for _, name := range []string{sharesCollection, reportsCollection} {
		col, err := s.app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.app.Delete(col); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := offboard.OffboardUser(s.app, s.id("ann"), offboard.Plan{Mode: offboard.ModeKeep}, s.id("ad")); err != nil {
		t.Fatal(err)
	}
}
