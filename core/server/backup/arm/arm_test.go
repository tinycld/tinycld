package arm

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"tinycld.org/core/backup/format"
)

func stagedDB(t *testing.T, pending string, rows int) {
	t.Helper()
	if err := os.MkdirAll(pending, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(pending, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE notes (id TEXT)"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < rows; i++ {
		if _, err := db.Exec("INSERT INTO notes VALUES ('x')"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestArmWritesSentinelThenMarker(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "pb_data")
	rd := Dir(dataDir)
	pending := PendingDir(rd, "job1")
	stagedDB(t, pending, 2)
	m := Marker{ID: "job1", Pending: pending, Manifest: format.Manifest{Counts: format.Counts{Collections: map[string]int{"notes": 2}}}}
	if err := Arm(rd, m); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(pending, StagedSentinel)); err != nil {
		t.Fatal("no sentinel")
	}
	raw, err := os.ReadFile(MarkerPath(rd))
	if err != nil {
		t.Fatal(err)
	}
	var got Marker
	if err := json.Unmarshal(raw, &got); err != nil || got.ID != "job1" {
		t.Fatalf("marker = %s", raw)
	}
	if rd != filepath.Join(root, "restore") {
		t.Fatalf("Dir = %s", rd)
	}
}

func TestArmRefusesWrongCounts(t *testing.T) {
	rd := Dir(filepath.Join(t.TempDir(), "pb_data"))
	pending := PendingDir(rd, "job1")
	stagedDB(t, pending, 1)
	m := Marker{ID: "job1", Pending: pending, Manifest: format.Manifest{Counts: format.Counts{Collections: map[string]int{"notes": 2}}}}
	if err := Arm(rd, m); err == nil {
		t.Fatal("armed a DB whose counts disagree with its manifest")
	}
	if _, err := os.Stat(MarkerPath(rd)); !os.IsNotExist(err) {
		t.Fatal("marker written after a failed check")
	}
}

func TestExtractTarRefusesEscapesAndLinks(t *testing.T) {
	for name, hdr := range map[string]*tar.Header{
		"escape":  {Name: "../x", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg},
		"symlink": {Name: "a", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink},
	} {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		_ = tw.WriteHeader(hdr)
		if hdr.Size > 0 {
			_, _ = tw.Write([]byte("x"))
		}
		_ = tw.Close()
		if err := ExtractTar(&buf, t.TempDir()); err == nil {
			t.Fatalf("%s: extracted", name)
		}
	}
}

func TestExtractTarWritesFiles(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: "c1/r1/a.txt", Mode: 0o644, Size: 5, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("hello"))
	_ = tw.Close()
	dir := t.TempDir()
	if err := ExtractTar(&buf, dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "c1", "r1", "a.txt"))
	if err != nil || string(got) != "hello" {
		t.Fatalf("%q %v", got, err)
	}
}
