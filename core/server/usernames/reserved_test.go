package usernames

import "testing"

func TestReserved(t *testing.T) {
	if !Reserved("deleted-abc") {
		t.Error("deleted-abc must be reserved")
	}
	if Reserved("deletedabc") || Reserved("ann") {
		t.Error("only the deleted- prefix is reserved")
	}
}
