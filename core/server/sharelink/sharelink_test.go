package sharelink

import (
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupApp builds the minimal collection graph sharelink touches:
// drive_items + drive_share_links. _superusers (used for the signing
// key) ships with NewTestApp().
func setupApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	driveItems := core.NewBaseCollection("drive_items")
	driveItems.Fields.Add(&core.TextField{Name: "name", Required: true})
	driveItems.Fields.Add(&core.TextField{Name: "mime_type"})
	if err := app.Save(driveItems); err != nil {
		t.Fatalf("save drive_items: %v", err)
	}

	links := core.NewBaseCollection("drive_share_links")
	links.Fields.Add(&core.RelationField{
		Name: "item", Required: true, CollectionId: driveItems.Id, MaxSelect: 1,
	})
	links.Fields.Add(&core.TextField{Name: "token", Required: true})
	links.Fields.Add(&core.SelectField{
		Name: "role", Required: true, MaxSelect: 1,
		Values: []string{RoleViewer, RoleCommentor, RoleEditor},
	})
	links.Fields.Add(&core.BoolField{Name: "is_active"})
	links.Fields.Add(&core.DateField{Name: "expires_at"})
	if err := app.Save(links); err != nil {
		t.Fatalf("save drive_share_links: %v", err)
	}
	return app
}

// makeLink inserts a drive_item + drive_share_links record and returns
// the 64-char token.
func makeLink(t *testing.T, app *tests.TestApp, role string, active bool, expires time.Time) (string, string) {
	t.Helper()
	itemsCol, err := app.FindCollectionByNameOrId("drive_items")
	if err != nil {
		t.Fatal(err)
	}
	item := core.NewRecord(itemsCol)
	item.Set("name", "sheet")
	item.Set("mime_type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err := app.Save(item); err != nil {
		t.Fatalf("save item: %v", err)
	}

	token := strings.Repeat("a", 64)
	// Make the token unique-ish per call so multiple links don't collide.
	token = token[:64-len(item.Id)] + item.Id

	linksCol, err := app.FindCollectionByNameOrId("drive_share_links")
	if err != nil {
		t.Fatal(err)
	}
	link := core.NewRecord(linksCol)
	link.Set("item", item.Id)
	link.Set("token", token)
	link.Set("role", role)
	link.Set("is_active", active)
	if !expires.IsZero() {
		link.Set("expires_at", expires.UTC().Format(time.RFC3339))
	}
	if err := app.Save(link); err != nil {
		t.Fatalf("save link: %v", err)
	}
	return token, item.Id
}

func TestMintAndVerifySession(t *testing.T) {
	app := setupApp(t)
	token, itemID := makeLink(t, app, RoleEditor, true, time.Time{})

	anonID := NewAnonID()
	claims := Claims{
		ShareToken:  token,
		AnonID:      anonID,
		DisplayName: DisplayName(anonID),
		Role:        RoleEditor,
		ItemID:      itemID,
	}
	tok, err := MintSession(app, claims)
	if err != nil {
		t.Fatalf("MintSession: %v", err)
	}

	got, err := VerifySession(app, tok)
	if err != nil {
		t.Fatalf("VerifySession: %v", err)
	}
	if got.AnonID != anonID || got.ShareToken != token || got.ItemID != itemID || got.Role != RoleEditor {
		t.Fatalf("claims mismatch: %+v", got)
	}
	if !got.CanEdit() || !got.CanComment() {
		t.Fatalf("editor should edit + comment: %+v", got)
	}
}

func TestVerifyRejectsTamper(t *testing.T) {
	app := setupApp(t)
	token, itemID := makeLink(t, app, RoleViewer, true, time.Time{})
	tok, err := MintSession(app, Claims{
		ShareToken: token, AnonID: NewAnonID(), Role: RoleViewer, ItemID: itemID,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Flip a character in the signature segment.
	tampered := tok[:len(tok)-3] + "xyz"
	if _, err := VerifySession(app, tampered); err == nil {
		t.Fatal("expected tampered token to be rejected")
	}
}

func TestVerifyAndResolveRevoked(t *testing.T) {
	app := setupApp(t)
	token, itemID := makeLink(t, app, RoleViewer, false /* revoked */, time.Time{})
	tok, err := MintSession(app, Claims{
		ShareToken: token, AnonID: NewAnonID(), Role: RoleViewer, ItemID: itemID,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = VerifyAndResolve(app, tok)
	if err == nil {
		t.Fatal("expected revoked link to fail resolve")
	}
	if HTTPStatus(err) != 410 {
		t.Fatalf("expected 410, got %d", HTTPStatus(err))
	}
}

func TestVerifyAndResolveExpired(t *testing.T) {
	app := setupApp(t)
	token, itemID := makeLink(t, app, RoleViewer, true, time.Now().Add(-time.Hour))
	tok, err := MintSession(app, Claims{
		ShareToken: token, AnonID: NewAnonID(), Role: RoleViewer, ItemID: itemID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := VerifyAndResolve(app, tok); HTTPStatus(err) != 410 {
		t.Fatalf("expected 410 for expired, got %v (%d)", err, HTTPStatus(err))
	}
}

func TestVerifyAndResolveRoleDowngrade(t *testing.T) {
	app := setupApp(t)
	// Token minted as editor, but the link is now only viewer.
	token, itemID := makeLink(t, app, RoleViewer, true, time.Time{})
	tok, err := MintSession(app, Claims{
		ShareToken: token, AnonID: NewAnonID(), Role: RoleEditor, ItemID: itemID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := VerifyAndResolve(app, tok); err == nil {
		t.Fatal("expected editor token against a viewer link to be rejected")
	}
}

func TestVerifyAndResolveHappy(t *testing.T) {
	app := setupApp(t)
	token, itemID := makeLink(t, app, RoleCommentor, true, time.Time{})
	tok, err := MintSession(app, Claims{
		ShareToken: token, AnonID: NewAnonID(), Role: RoleCommentor, ItemID: itemID,
	})
	if err != nil {
		t.Fatal(err)
	}
	claims, link, item, err := VerifyAndResolve(app, tok)
	if err != nil {
		t.Fatalf("VerifyAndResolve: %v", err)
	}
	if item.Id != itemID || link.GetString("token") != token {
		t.Fatal("resolved wrong records")
	}
	if !claims.CanComment() || claims.CanEdit() {
		t.Fatalf("commentor should comment but not edit: %+v", claims)
	}
}

func TestResolveLinkNotFound(t *testing.T) {
	app := setupApp(t)
	if _, _, err := ResolveLink(app, strings.Repeat("z", 64)); HTTPStatus(err) != 404 {
		t.Fatalf("expected 404, got %v", err)
	}
	// Wrong length.
	if _, _, err := ResolveLink(app, "short"); HTTPStatus(err) != 404 {
		t.Fatalf("expected 404 for short token, got %v", err)
	}
}

func TestDisplayNameStable(t *testing.T) {
	id := NewAnonID()
	a := DisplayName(id)
	b := DisplayName(id)
	if a != b {
		t.Fatalf("display name not stable: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "Anon ") {
		t.Fatalf("expected Anon prefix: %q", a)
	}
	// Different ids should (almost always) differ.
	if DisplayName(NewAnonID()) == a && DisplayName(NewAnonID()) == a {
		t.Fatal("display names suspiciously identical across ids")
	}
}

func TestIsValidAnonID(t *testing.T) {
	if !IsValidAnonID(NewAnonID()) {
		t.Fatal("minted id should be valid")
	}
	for _, bad := range []string{"", "anon_", "nope", "anon_!!!", "anon_" + strings.Repeat("a", 21)} {
		if IsValidAnonID(bad) {
			t.Fatalf("expected %q invalid", bad)
		}
	}
}
