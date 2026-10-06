package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Deleting a model with files removes its storage dir in the background.
// ClearBootstrap must wait for that delete: otherwise it still runs after
// the app is torn down, and it recreates <dataDir>/storage while the caller
// removes the data dir.
func TestClearBootstrap_WaitsForBackgroundFileDeletes(t *testing.T) {
	app := NewBaseApp(BaseAppConfig{DataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	col := NewBaseCollection("docs")
	col.Fields.Add(&FileField{Name: "file", MaxSelect: 1})
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join(app.DataDir(), LocalStorageDirName, col.BaseFilesPath(), "a.txt")
	if err := os.MkdirAll(filepath.Dir(stored), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stored, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	entered, release := make(chan struct{}), make(chan struct{})
	OnFilesystemDelete(app).BindFunc(func(e *FilesystemDeleteEvent) error {
		close(entered)
		<-release
		return e.Next()
	})

	if err := app.Delete(col); err != nil {
		close(release)
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("the background file delete never started")
	}

	cleared := make(chan error, 1)
	go func() { cleared <- app.ClearBootstrap() }()
	select {
	case <-cleared:
		close(release)
		t.Fatal("ClearBootstrap returned while a background file delete was still running")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-cleared:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ClearBootstrap did not finish after the file delete")
	}
	if _, err := os.Stat(stored); !os.IsNotExist(err) {
		t.Fatalf("stat %s after ClearBootstrap = %v; want the file deleted", stored, err)
	}
}
