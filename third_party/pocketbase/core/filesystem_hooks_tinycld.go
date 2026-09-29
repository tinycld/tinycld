package core

import "github.com/pocketbase/pocketbase/tools/hook"

// OnFilesystemDelete exposes the internal hook that app.NewFilesystem() binds
// to every storage delete. A handler that returns without calling e.Next()
// skips the delete. tinycld's backup delete hold uses it to keep every file a
// database snapshot refers to until the snapshot's files are read.
//
// Bind it before the first app.NewFilesystem() call you want covered: the
// filesystem attaches the hook only when a handler exists at creation time.
func OnFilesystemDelete(app App) *hook.Hook[*FilesystemDeleteEvent] {
	return app.onFilesystemDelete()
}
