package backup

import (
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/tools/filesystem"

	"tinycld.org/core/backup/hold"
)

func TestHeldDeleteIsJournaledThenDrained(t *testing.T) {
	app := newTestApp(t)
	BindDeleteHold(app)

	fs, err := app.NewFilesystem()
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()

	h, err := hold.Acquire(app.DataDir(), "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Delete("col1/rec1/hello.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Attributes("col1/rec1/hello.txt"); err != nil {
		t.Fatalf("file gone under a hold: %v", err)
	}

	DrainHeldDeletes(app)
	if _, err := fs.Attributes("col1/rec1/hello.txt"); err != nil {
		t.Fatal("drained while the hold was valid")
	}

	if err := h.Release(); err != nil {
		t.Fatal(err)
	}
	DrainHeldDeletes(app)
	if _, err := fs.Attributes("col1/rec1/hello.txt"); !errors.Is(err, filesystem.ErrNotFound) {
		t.Fatalf("file not deleted after release: %v", err)
	}
}

func TestExpiredHoldDoesNotBlockDeletes(t *testing.T) {
	app := newTestApp(t)
	BindDeleteHold(app)
	past := func() time.Time { return time.Now().Add(-2 * hold.Lease) }
	if _, err := hold.Acquire(app.DataDir(), "crashed", past); err != nil {
		t.Fatal(err)
	}
	fs, _ := app.NewFilesystem()
	defer fs.Close()
	if err := fs.Delete("col1/rec1/hello.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Attributes("col1/rec1/hello.txt"); !errors.Is(err, filesystem.ErrNotFound) {
		t.Fatal("an expired hold still blocked the delete")
	}
}
