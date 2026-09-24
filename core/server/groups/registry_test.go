package groups

import "testing"

func TestRegisterGrantTableIsIdempotentAndIgnoresBlanks(t *testing.T) {
	ResetForTesting()
	RegisterGrantTable(GrantTable{Collection: "zoo_keepers", ResourceField: "zoo"})
	RegisterGrantTable(GrantTable{Collection: "zoo_keepers", ResourceField: "zoo"})
	RegisterGrantTable(GrantTable{Collection: "", ResourceField: "zoo"})
	RegisterGrantTable(GrantTable{Collection: "zoo_keepers", ResourceField: ""})

	got := RegisteredGrantTables()
	if len(got) != 1 || got[0].Collection != "zoo_keepers" || got[0].ResourceField != "zoo" {
		t.Fatalf("registry = %+v, want one zoo_keepers entry", got)
	}
}

func TestMembershipListenersRunInOrderAndStopOnError(t *testing.T) {
	ResetForTesting()
	var seen []string
	OnMembershipChange(func(e MembershipEvent) error {
		seen = append(seen, "a:"+e.UserID)
		return nil
	})
	OnMembershipChange(func(e MembershipEvent) error {
		seen = append(seen, "b:"+e.UserID)
		return errStop
	})
	OnMembershipChange(func(e MembershipEvent) error {
		seen = append(seen, "c:"+e.UserID)
		return nil
	})
	err := notifyMembership(MembershipEvent{UserID: "u1", GroupID: "g1", Joined: true})
	if err == nil {
		t.Fatal("expected the second listener's error to surface")
	}
	if len(seen) != 2 || seen[0] != "a:u1" || seen[1] != "b:u1" {
		t.Fatalf("seen = %v", seen)
	}
}
