package core_test

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

// A handler that does not call e.Next() must stop the delete. The backup
// delete hold depends on exactly that.
func TestOnFilesystemDeleteCanSkipTheDelete(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	var seen string
	core.OnFilesystemDelete(app).BindFunc(func(e *core.FilesystemDeleteEvent) error {
		seen = e.FileKey
		return nil
	})

	fs, err := app.NewFilesystem()
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	if err := fs.Upload([]byte("x"), "held/a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Delete("held/a.txt"); err != nil {
		t.Fatal(err)
	}
	if seen != "held/a.txt" {
		t.Fatalf("hook saw %q", seen)
	}
	if _, err := fs.Attributes("held/a.txt"); errors.Is(err, filesystem.ErrNotFound) {
		t.Fatal("the file was deleted although the handler skipped e.Next()")
	}
}
