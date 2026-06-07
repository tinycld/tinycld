package coreserver

import "testing"

func TestSolveCompatSatisfied(t *testing.T) {
	resolved := map[string]string{
		"mail":         "1.2.0",
		"contacts":     "1.0.0",
		corePackageKey: "2.5.0",
	}
	peers := map[string]map[string]string{
		"mail": {corePackageKey: ">=2.1 <3", "contacts": ">=1.0"},
	}
	if v := solveCompat(resolved, peers); len(v) != 0 {
		t.Fatalf("expected no violations, got %v", v)
	}
}

func TestSolveCompatViolatedRange(t *testing.T) {
	resolved := map[string]string{
		"mail":         "1.2.0",
		corePackageKey: "2.0.0", // below the required >=2.1
	}
	peers := map[string]map[string]string{
		"mail": {corePackageKey: ">=2.1 <3"},
	}
	v := solveCompat(resolved, peers)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(v), v)
	}
	if v[0].Requires != corePackageKey || v[0].Found != "2.0.0" {
		t.Errorf("violation = %+v, want requires %s found 2.0.0", v[0], corePackageKey)
	}
}

func TestSolveCompatMissingPeer(t *testing.T) {
	resolved := map[string]string{"mail": "1.2.0"} // contacts not installed
	peers := map[string]map[string]string{
		"mail": {"contacts": ">=1.0"},
	}
	v := solveCompat(resolved, peers)
	if len(v) != 1 || v[0].Found != "" {
		t.Fatalf("expected 1 violation with empty Found (absent peer), got %v", v)
	}
}

func TestSolveCompatUnparsableRangeIsViolation(t *testing.T) {
	resolved := map[string]string{"mail": "1.2.0", "contacts": "1.0.0"}
	peers := map[string]map[string]string{
		"mail": {"contacts": "not-a-range"},
	}
	if v := solveCompat(resolved, peers); len(v) != 1 {
		t.Fatalf("expected unparsable range to be a violation, got %v", v)
	}
}

func TestPeerVersionsFromManifest(t *testing.T) {
	json := `{"slug":"mail","peerVersions":{"@tinycld/core":">=2.1","contacts":">=1.0"}}`
	peers := peerVersionsFromManifest(json)
	if peers["@tinycld/core"] != ">=2.1" || peers["contacts"] != ">=1.0" {
		t.Errorf("peerVersionsFromManifest = %v, want core>=2.1 contacts>=1.0", peers)
	}
	if peerVersionsFromManifest("") != nil {
		t.Error("empty manifest should yield nil")
	}
	if peerVersionsFromManifest("not json") != nil {
		t.Error("malformed manifest should yield nil")
	}
}
