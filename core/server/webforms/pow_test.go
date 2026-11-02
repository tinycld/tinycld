package webforms

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

func solve(c Challenge) Solution {
	for n := 0; n <= c.MaxNumber; n++ {
		sum := sha256.Sum256([]byte(c.Salt + strconv.Itoa(n)))
		if hex.EncodeToString(sum[:]) == c.Challenge {
			return Solution{Algorithm: c.Algorithm, Challenge: c.Challenge, Number: n, Salt: c.Salt, Signature: c.Signature}
		}
	}
	return Solution{}
}

func TestPow(t *testing.T) {
	key := []byte("k")
	t0 := time.Unix(1_800_000_000, 0)
	c, err := newChallenge(key, "pub1", t0)
	if err != nil {
		t.Fatal(err)
	}
	s := solve(c)
	l := newPowLedger()
	if err := l.verify(key, s, "pub1", t0.Add(time.Minute)); err != nil {
		t.Fatalf("valid solution refused: %v", err)
	}
	if err := l.verify(key, s, "pub1", t0.Add(time.Minute)); err == nil {
		t.Fatal("a solution must be single use")
	}
	c2, _ := newChallenge(key, "pub1", t0)
	s2 := solve(c2)
	if err := newPowLedger().verify(key, s2, "pub2", t0.Add(time.Minute)); err == nil {
		t.Fatal("a solution for another publication must fail")
	}
	if err := newPowLedger().verify(key, s2, "pub1", t0.Add(11*time.Minute)); err == nil {
		t.Fatal("an expired challenge must fail")
	}
	bad := s2
	bad.Number++
	if err := newPowLedger().verify(key, bad, "pub1", t0.Add(time.Minute)); err == nil {
		t.Fatal("a wrong number must fail")
	}
	if err := newPowLedger().verify([]byte("other"), s2, "pub1", t0.Add(time.Minute)); err == nil {
		t.Fatal("a forged signature must fail")
	}
}
