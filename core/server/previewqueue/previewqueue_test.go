package previewqueue

import "testing"

func TestStashThenTakeIsSelfClearing(t *testing.T) {
	ResetForTest()

	want := Payload{Plaintext: "hello", RegionHash: "rh", IndexHash: "ih"}
	Stash("item-1", want)

	got, ok := Take("item-1")
	if !ok {
		t.Fatal("first Take: ok = false, want true")
	}
	// Payload embeds a PageModel with slice fields, so it isn't comparable
	// with ==; assert the scalar fields we stashed instead.
	if got.Plaintext != want.Plaintext || got.RegionHash != want.RegionHash || got.IndexHash != want.IndexHash {
		t.Fatalf("first Take payload = %+v, want %+v", got, want)
	}

	if _, ok := Take("item-1"); ok {
		t.Fatal("second Take: ok = true, want false (Take should self-clear)")
	}
}

func TestTakeUnknownID(t *testing.T) {
	ResetForTest()

	if _, ok := Take("does-not-exist"); ok {
		t.Fatal("Take of unknown id: ok = true, want false")
	}
}

func TestStashTwiceOverwrites(t *testing.T) {
	ResetForTest()

	Stash("item-1", Payload{Plaintext: "first"})
	Stash("item-1", Payload{Plaintext: "second"})

	got, ok := Take("item-1")
	if !ok {
		t.Fatal("Take: ok = false, want true")
	}
	if got.Plaintext != "second" {
		t.Fatalf("payload = %q, want last-write %q", got.Plaintext, "second")
	}
}
