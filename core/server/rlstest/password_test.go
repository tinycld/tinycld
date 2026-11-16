package rlstest

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/guestauth"
	"tinycld.org/core/usernames"
)

func TestSetPasswordIsAcceptedAndSaves(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	var hashes []string
	for _, email := range []string{"a@test.local", "b@test.local"} {
		username, err := guestauth.PickAvailableUsername(app, usernames.Derive(email))
		if err != nil {
			t.Fatal(err)
		}
		r := core.NewRecord(users)
		r.SetEmail(email)
		r.Set("username", username)
		SetPassword(r, "Password123!")
		if err := app.Save(r); err != nil {
			t.Fatalf("save %s: %v", email, err)
		}
		saved, err := app.FindAuthRecordByEmail(users, email)
		if err != nil {
			t.Fatal(err)
		}
		if !saved.ValidatePassword("Password123!") {
			t.Fatalf("%s: saved password does not validate", email)
		}
		if saved.ValidatePassword("wrong") {
			t.Fatalf("%s: a wrong password validates", email)
		}
		hashes = append(hashes, saved.GetString(core.FieldNamePassword+":hash"))
	}
	if hashes[0] != hashes[1] {
		t.Fatal("the same password was hashed twice")
	}
}
