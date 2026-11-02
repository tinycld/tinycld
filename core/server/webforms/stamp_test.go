package webforms

import (
	"errors"
	"testing"
	"time"
)

func TestStamp(t *testing.T) {
	key := []byte("k")
	t0 := time.Unix(1_800_000_000, 0)
	s := issueStamp(key, "pub1", t0)

	if err := checkStamp(key, s, "pub1", t0.Add(4*time.Second)); err != nil {
		t.Fatalf("valid stamp refused: %v", err)
	}
	if err := checkStamp(key, s, "pub1", t0.Add(time.Second)); !errors.Is(err, errTooFast) {
		t.Fatalf("want errTooFast, got %v", err)
	}
	if err := checkStamp(key, s, "pub1", t0.Add(3*time.Hour)); !errors.Is(err, errStamp) {
		t.Fatalf("want errStamp for an old stamp, got %v", err)
	}
	if err := checkStamp(key, s, "pub2", t0.Add(4*time.Second)); !errors.Is(err, errStamp) {
		t.Fatalf("want errStamp for another publication, got %v", err)
	}
	if err := checkStamp([]byte("other"), s, "pub1", t0.Add(4*time.Second)); !errors.Is(err, errStamp) {
		t.Fatalf("want errStamp for a forged signature, got %v", err)
	}
	if err := checkStamp(key, "garbage", "pub1", t0); !errors.Is(err, errStamp) {
		t.Fatalf("want errStamp for garbage, got %v", err)
	}
}
