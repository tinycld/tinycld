package notify

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupCommentMentionTestApp builds the minimum collection graph the
// notify hook touches: users, drive_items, text_comments, comment_mentions,
// and notifications. NewTestApp() ships the standard PB users + collections
// (settings, etc.) but not the tinycld extensions, so we add them inline.
//
// Single-org: there are no orgs/user_org collections. mentioned_user and
// text_comments.author keep their field names but hold users ids directly.
func setupCommentMentionTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	// Set an AppURL so the hook can build absolute deep links.
	settings := app.Settings()
	settings.Meta.AppURL = "https://app.test.local"
	if err := app.Save(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	users.Fields.Add(&core.BoolField{Name: "is_demo"})
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}

	driveItems := core.NewBaseCollection("drive_items")
	driveItems.Fields.Add(&core.TextField{Name: "name", Required: true})
	driveItems.Fields.Add(&core.RelationField{
		Name: "created_by", CollectionId: users.Id, MaxSelect: 1,
	})
	if err := app.Save(driveItems); err != nil {
		t.Fatal(err)
	}

	textComments := core.NewBaseCollection("text_comments")
	textComments.Fields.Add(&core.RelationField{
		Name: "drive_item", Required: true, CollectionId: driveItems.Id, MaxSelect: 1,
	})
	textComments.Fields.Add(&core.TextField{Name: "comment_id"})
	textComments.Fields.Add(&core.TextField{Name: "quoted_text"})
	textComments.Fields.Add(&core.TextField{Name: "parent_comment"})
	textComments.Fields.Add(&core.TextField{Name: "body"})
	textComments.Fields.Add(&core.TextField{Name: "resolved_at"})
	textComments.Fields.Add(&core.RelationField{
		Name: "author", Required: true, CollectionId: users.Id, MaxSelect: 1,
	})
	textComments.Fields.Add(&core.TextField{Name: "author_name"})
	textComments.Fields.Add(&core.TextField{Name: "suggestion_id"})
	textComments.Fields.Add(&core.TextField{Name: "archived_at"})
	if err := app.Save(textComments); err != nil {
		t.Fatal(err)
	}

	commentMentions := core.NewBaseCollection("comment_mentions")
	commentMentions.Fields.Add(&core.TextField{Name: "comment_collection", Required: true})
	commentMentions.Fields.Add(&core.TextField{Name: "comment_record", Required: true})
	commentMentions.Fields.Add(&core.RelationField{
		Name: "drive_item", Required: true, CollectionId: driveItems.Id, MaxSelect: 1,
	})
	commentMentions.Fields.Add(&core.RelationField{
		Name: "mentioned_user", Required: true, CollectionId: users.Id, MaxSelect: 1,
	})
	if err := app.Save(commentMentions); err != nil {
		t.Fatal(err)
	}

	notifications := core.NewBaseCollection("notifications")
	notifications.Fields.Add(&core.RelationField{
		Name: "user", Required: true, CollectionId: users.Id, MaxSelect: 1,
	})
	notifications.Fields.Add(&core.TextField{Name: "type"})
	notifications.Fields.Add(&core.TextField{Name: "package"})
	notifications.Fields.Add(&core.TextField{Name: "title"})
	notifications.Fields.Add(&core.TextField{Name: "body"})
	notifications.Fields.Add(&core.TextField{Name: "url"})
	notifications.Fields.Add(&core.JSONField{Name: "metadata"})
	notifications.Fields.Add(&core.BoolField{Name: "read"})
	notifications.Fields.Add(&core.BoolField{Name: "dismissed"})
	if err := app.Save(notifications); err != nil {
		t.Fatal(err)
	}

	return app
}

type mentionFixture struct {
	app         *tests.TestApp
	authorUser  *core.Record
	mentionUser *core.Record
	driveItem   *core.Record
	commentRoot *core.Record
}

func seedMentionFixture(t *testing.T) *mentionFixture {
	t.Helper()
	app := setupCommentMentionTestApp(t)

	usersCol, _ := app.FindCollectionByNameOrId("users")
	authorUser := core.NewRecord(usersCol)
	authorUser.SetEmail("author@test.local")
	authorUser.Set("name", "Alice")
	authorUser.SetVerified(true)
	authorUser.SetPassword("Password123!")
	if err := app.Save(authorUser); err != nil {
		t.Fatal(err)
	}

	mentionUser := core.NewRecord(usersCol)
	mentionUser.SetEmail("mention@test.local")
	mentionUser.Set("name", "Bob")
	mentionUser.SetVerified(true)
	mentionUser.SetPassword("Password123!")
	if err := app.Save(mentionUser); err != nil {
		t.Fatal(err)
	}

	driveCol, _ := app.FindCollectionByNameOrId("drive_items")
	driveItem := core.NewRecord(driveCol)
	driveItem.Set("name", "doc.txt")
	driveItem.Set("created_by", authorUser.Id)
	if err := app.Save(driveItem); err != nil {
		t.Fatal(err)
	}

	tcCol, _ := app.FindCollectionByNameOrId("text_comments")
	commentRoot := core.NewRecord(tcCol)
	commentRoot.Set("drive_item", driveItem.Id)
	commentRoot.Set("comment_id", "cm_xyz")
	commentRoot.Set("parent_comment", "")
	commentRoot.Set("body", "hi [[@"+mentionUser.Id+"]]")
	commentRoot.Set("author", authorUser.Id)
	commentRoot.Set("author_name", "Alice")
	if err := app.Save(commentRoot); err != nil {
		t.Fatal(err)
	}

	return &mentionFixture{
		app:         app,
		authorUser:  authorUser,
		mentionUser: mentionUser,
		driveItem:   driveItem,
		commentRoot: commentRoot,
	}
}

// runHookSync invokes handleCommentMention directly (bypassing the
// goroutine in the registered hook) so tests don't have to race on a
// time.Sleep. The hook itself is just `go handleCommentMention(...)`
// so this preserves the same code path.
func runHookSync(t *testing.T, app core.App, mention *core.Record) {
	t.Helper()
	handleCommentMention(app, mention)
}

// findLatestNotification returns the single notification record for
// the user, or nil if none exists. Fails the test if multiple are
// present — every test should be in a clean state.
func findLatestNotification(t *testing.T, app core.App, userID string) *core.Record {
	t.Helper()
	records, err := app.FindRecordsByFilter(
		"notifications",
		"user = {:userId}",
		"",
		10, 0,
		map[string]any{"userId": userID},
	)
	if err != nil {
		t.Fatalf("find notifications: %v", err)
	}
	if len(records) == 0 {
		return nil
	}
	if len(records) > 1 {
		t.Fatalf("expected at most one notification, got %d", len(records))
	}
	return records[0]
}

func mkMention(t *testing.T, app core.App, f *mentionFixture, collection string) *core.Record {
	t.Helper()
	cmCol, _ := app.FindCollectionByNameOrId("comment_mentions")
	mention := core.NewRecord(cmCol)
	mention.Set("comment_collection", collection)
	mention.Set("comment_record", f.commentRoot.Id)
	mention.Set("drive_item", f.driveItem.Id)
	mention.Set("mentioned_user", f.mentionUser.Id)
	if err := app.Save(mention); err != nil {
		t.Fatal(err)
	}
	return mention
}

func TestCommentMention_AllowlistRejectsUnknownCollection(t *testing.T) {
	f := seedMentionFixture(t)
	mention := mkMention(t, f.app, f, "bogus_collection")
	runHookSync(t, f.app, mention)
	if got := findLatestNotification(t, f.app, f.mentionUser.Id); got != nil {
		t.Errorf("expected no notification for unknown collection, got %v", got.Id)
	}
}

func TestCommentMention_HappyPathWritesNotification(t *testing.T) {
	f := seedMentionFixture(t)
	mention := mkMention(t, f.app, f, "text_comments")
	runHookSync(t, f.app, mention)
	n := findLatestNotification(t, f.app, f.mentionUser.Id)
	if n == nil {
		t.Fatal("expected notification, got none")
	}
	if got := n.GetString("type"); got != "comment_mention" {
		t.Errorf("type = %q, want comment_mention", got)
	}
	if got := n.GetString("package"); got != "text" {
		t.Errorf("package = %q, want text", got)
	}
	wantURL := "https://app.test.local/p/text/" + f.driveItem.Id + "?thread=" + f.commentRoot.Id
	if got := n.GetString("url"); got != wantURL {
		t.Errorf("url = %q, want %q", got, wantURL)
	}
}

func TestCommentMention_ReplyDeepLinksToRootThread(t *testing.T) {
	f := seedMentionFixture(t)

	// Create a reply pointing at the root. Mentions on a reply should
	// deep-link to the root (so the drawer opens with the whole thread
	// in view), not to the reply id.
	tcCol, _ := f.app.FindCollectionByNameOrId("text_comments")
	reply := core.NewRecord(tcCol)
	reply.Set("drive_item", f.driveItem.Id)
	reply.Set("comment_id", "cm_xyz")
	reply.Set("parent_comment", f.commentRoot.Id)
	reply.Set("body", "ping [[@"+f.mentionUser.Id+"]]")
	reply.Set("author", f.authorUser.Id)
	reply.Set("author_name", "Alice")
	if err := f.app.Save(reply); err != nil {
		t.Fatal(err)
	}

	cmCol, _ := f.app.FindCollectionByNameOrId("comment_mentions")
	mention := core.NewRecord(cmCol)
	mention.Set("comment_collection", "text_comments")
	mention.Set("comment_record", reply.Id)
	mention.Set("drive_item", f.driveItem.Id)
	mention.Set("mentioned_user", f.mentionUser.Id)
	if err := f.app.Save(mention); err != nil {
		t.Fatal(err)
	}

	runHookSync(t, f.app, mention)

	n := findLatestNotification(t, f.app, f.mentionUser.Id)
	if n == nil {
		t.Fatal("expected notification, got none")
	}
	wantURL := "https://app.test.local/p/text/" + f.driveItem.Id + "?thread=" + f.commentRoot.Id
	if got := n.GetString("url"); got != wantURL {
		t.Errorf("url = %q, want %q (reply should deep-link to root)", got, wantURL)
	}
}

func TestCommentMention_SuggestionReplyDeepLinksWithFocusSuggestionParam(t *testing.T) {
	f := seedMentionFixture(t)

	// Create a suggestion-reply row: text_comments with suggestion_id set.
	// The notify hook should detect the suggestion_id and emit a
	// ?focusSuggestion=<id> URL instead of ?thread=<thread>, so the
	// recipient lands on the focused suggestion row in the review drawer.
	tcCol, _ := f.app.FindCollectionByNameOrId("text_comments")
	suggestionReply := core.NewRecord(tcCol)
	suggestionReply.Set("drive_item", f.driveItem.Id)
	suggestionReply.Set("comment_id", "synth_xyz")
	suggestionReply.Set("parent_comment", "")
	suggestionReply.Set("body", "ping [[@"+f.mentionUser.Id+"]]")
	suggestionReply.Set("author", f.authorUser.Id)
	suggestionReply.Set("author_name", "Alice")
	suggestionReply.Set("suggestion_id", "sug_abc123")
	if err := f.app.Save(suggestionReply); err != nil {
		t.Fatal(err)
	}

	cmCol, _ := f.app.FindCollectionByNameOrId("comment_mentions")
	mention := core.NewRecord(cmCol)
	mention.Set("comment_collection", "text_comments")
	mention.Set("comment_record", suggestionReply.Id)
	mention.Set("drive_item", f.driveItem.Id)
	mention.Set("mentioned_user", f.mentionUser.Id)
	if err := f.app.Save(mention); err != nil {
		t.Fatal(err)
	}

	runHookSync(t, f.app, mention)

	n := findLatestNotification(t, f.app, f.mentionUser.Id)
	if n == nil {
		t.Fatal("expected notification, got none")
	}
	wantURL := "https://app.test.local/p/text/" + f.driveItem.Id + "?focusSuggestion=sug_abc123"
	if got := n.GetString("url"); got != wantURL {
		t.Errorf("url = %q, want %q (suggestion reply should deep-link with focusSuggestion param)", got, wantURL)
	}
}

func TestCommentMention_SkipsSelfMention(t *testing.T) {
	f := seedMentionFixture(t)

	// Author mentions themselves. The client-side mutations factory
	// already drops these, but defense in depth lives here too.
	cmCol, _ := f.app.FindCollectionByNameOrId("comment_mentions")
	mention := core.NewRecord(cmCol)
	mention.Set("comment_collection", "text_comments")
	mention.Set("comment_record", f.commentRoot.Id)
	mention.Set("drive_item", f.driveItem.Id)
	mention.Set("mentioned_user", f.authorUser.Id)
	if err := f.app.Save(mention); err != nil {
		t.Fatal(err)
	}
	runHookSync(t, f.app, mention)

	if got := findLatestNotification(t, f.app, f.authorUser.Id); got != nil {
		t.Errorf("expected no self-mention notification, got %v", got.Id)
	}
}

func TestCommentMention_HookIsRegisteredAndFiresAsync(t *testing.T) {
	// Smoke test: the hook should fire via OnRecordAfterCreateSuccess.
	// It runs async (goroutine), so this test gives it a brief window
	// to land. The other tests use the sync helper so they don't have
	// to race; this one exercises the actual registration path.
	f := seedMentionFixture(t)
	registerCommentMentionHooksCore(f.app)

	cmCol, _ := f.app.FindCollectionByNameOrId("comment_mentions")
	mention := core.NewRecord(cmCol)
	mention.Set("comment_collection", "text_comments")
	mention.Set("comment_record", f.commentRoot.Id)
	mention.Set("drive_item", f.driveItem.Id)
	mention.Set("mentioned_user", f.mentionUser.Id)
	if err := f.app.Save(mention); err != nil {
		t.Fatal(err)
	}

	// Poll briefly so the goroutine has a chance to land. Plain
	// time.Sleep would be flaky on slow CI; loop with a backoff and
	// give up after ~1s.
	deadline := time.Now().Add(time.Second)
	var n *core.Record
	for time.Now().Before(deadline) {
		records, err := f.app.FindRecordsByFilter(
			"notifications",
			"user = {:userId}",
			"",
			10, 0,
			map[string]any{"userId": f.mentionUser.Id},
		)
		if err == nil && len(records) > 0 {
			n = records[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n == nil {
		t.Fatal("expected notification to be written by registered hook within 1s")
	}
}

// --- polymorphic target (core 1985000002) ---
//
// `comment_mentions` is no longer drive-only: it carries target_collection /
// target_record so a mention can hang off any package's record. These tests
// cover the cards path (no drive item at all) and the legacy fallback.

// addCardsComments extends the fixture app with the cards_comments table the
// cards path resolves its author through. Mirrors the real collection's
// shape only as far as this hook reads it.
func addCardsComments(t *testing.T, app core.App, usersID string) *core.Collection {
	t.Helper()
	c := core.NewBaseCollection("cards_comments")
	c.Fields.Add(&core.TextField{Name: "card"})
	c.Fields.Add(&core.TextField{Name: "project"})
	c.Fields.Add(&core.TextField{Name: "body"})
	c.Fields.Add(&core.TextField{Name: "parent_comment"})
	c.Fields.Add(&core.RelationField{
		Name: "author", Required: true, CollectionId: usersID, MaxSelect: 1,
	})
	c.Fields.Add(&core.TextField{Name: "author_name"})
	if err := app.Save(c); err != nil {
		t.Fatal(err)
	}
	return c
}

// seedCardsMention builds a cards comment + a mention row pointing at a CARD,
// with drive_item deliberately empty — the shape drive's schema used to forbid.
func seedCardsMention(t *testing.T, f *mentionFixture, cardID string) *core.Record {
	t.Helper()
	usersCol, _ := f.app.FindCollectionByNameOrId("users")
	cardsCol := addCardsComments(t, f.app, usersCol.Id)

	comment := core.NewRecord(cardsCol)
	comment.Set("card", cardID)
	comment.Set("project", "proj0000000000")
	comment.Set("body", "ping [[@"+f.mentionUser.Id+"]]")
	comment.Set("author", f.authorUser.Id)
	comment.Set("author_name", "Alice")
	if err := f.app.Save(comment); err != nil {
		t.Fatal(err)
	}

	cmCol, _ := f.app.FindCollectionByNameOrId("comment_mentions")
	mention := core.NewRecord(cmCol)
	mention.Set("comment_collection", "cards_comments")
	mention.Set("comment_record", comment.Id)
	mention.Set("target_collection", "cards_cards")
	mention.Set("target_record", cardID)
	mention.Set("mentioned_user", f.mentionUser.Id)
	// drive_item intentionally NOT set.
	if err := f.app.Save(mention); err != nil {
		t.Fatal(err)
	}
	return mention
}

func TestCommentMention_CardsTargetNotifiesWithoutDriveItem(t *testing.T) {
	f := seedMentionFixture(t)
	// The drive_item relation is required in this fixture's inline schema, so
	// relax it the way core 1985000002 does before inserting a cards row.
	relaxFixtureDriveItem(t, f.app)

	const cardID = "card0000000000"
	mention := seedCardsMention(t, f, cardID)
	runHookSync(t, f.app, mention)

	n := findLatestNotification(t, f.app, f.mentionUser.Id)
	if n == nil {
		t.Fatal("expected a notification for a cards mention, got none")
	}
	if got := n.GetString("package"); got != "cards" {
		t.Errorf("package = %q, want cards", got)
	}
	// Cards routes its board at /a/cards and opens a card via ?focused=.
	wantURL := "https://app.test.local/a/cards?focused=" + cardID
	if got := n.GetString("url"); got != wantURL {
		t.Errorf("url = %q, want %q", got, wantURL)
	}
}

// A cards author who mentions themselves must not be notified — the same
// defense-in-depth check the drive path gets.
func TestCommentMention_CardsSkipsSelfMention(t *testing.T) {
	f := seedMentionFixture(t)
	relaxFixtureDriveItem(t, f.app)

	usersCol, _ := f.app.FindCollectionByNameOrId("users")
	cardsCol := addCardsComments(t, f.app, usersCol.Id)
	comment := core.NewRecord(cardsCol)
	comment.Set("card", "card0000000000")
	comment.Set("body", "talking to myself")
	// Author IS the mentioned user.
	comment.Set("author", f.mentionUser.Id)
	comment.Set("author_name", "Bob")
	if err := f.app.Save(comment); err != nil {
		t.Fatal(err)
	}

	cmCol, _ := f.app.FindCollectionByNameOrId("comment_mentions")
	mention := core.NewRecord(cmCol)
	mention.Set("comment_collection", "cards_comments")
	mention.Set("comment_record", comment.Id)
	mention.Set("target_collection", "cards_cards")
	mention.Set("target_record", "card0000000000")
	mention.Set("mentioned_user", f.mentionUser.Id)
	if err := f.app.Save(mention); err != nil {
		t.Fatal(err)
	}

	runHookSync(t, f.app, mention)
	if got := findLatestNotification(t, f.app, f.mentionUser.Id); got != nil {
		t.Errorf("expected no notification for a self-mention, got %v", got.Id)
	}
}

// A row written by an older client carries only `drive_item`. The hook must
// still resolve it, or the migration window silently drops notifications.
func TestCommentMention_LegacyRowWithoutTargetColumns(t *testing.T) {
	f := seedMentionFixture(t)
	mention := mkMention(t, f.app, f, "text_comments") // sets drive_item only
	runHookSync(t, f.app, mention)

	n := findLatestNotification(t, f.app, f.mentionUser.Id)
	if n == nil {
		t.Fatal("expected a notification for a legacy drive-only row, got none")
	}
	wantURL := "https://app.test.local/p/text/" + f.driveItem.Id + "?thread=" + f.commentRoot.Id
	if got := n.GetString("url"); got != wantURL {
		t.Errorf("url = %q, want %q", got, wantURL)
	}
}

// relaxFixtureDriveItem mirrors what core migration 1985000002 does to the
// real table, so the inline fixture can hold a row with no drive item.
func relaxFixtureDriveItem(t *testing.T, app core.App) {
	t.Helper()
	cm, err := app.FindCollectionByNameOrId("comment_mentions")
	if err != nil {
		t.Fatal(err)
	}
	f := cm.Fields.GetByName("drive_item")
	if rel, ok := f.(*core.RelationField); ok {
		rel.Required = false
	}
	cm.Fields.Add(&core.TextField{Name: "target_collection", Max: 64})
	cm.Fields.Add(&core.TextField{Name: "target_record", Max: 32})
	if err := app.Save(cm); err != nil {
		t.Fatal(err)
	}
}

// The notification TYPE is what the per-user mute preference is keyed on, so a
// package's two mention sources must agree. Cards' DESCRIPTION mentions come
// from its own flush hook and send "cards_mention"; a cards COMMENT mention
// must send the same, or muting cards would silence only half of them.
func TestCommentMention_CardsUsesPackageScopedType(t *testing.T) {
	f := seedMentionFixture(t)
	relaxFixtureDriveItem(t, f.app)

	mention := seedCardsMention(t, f, "card0000000000")
	runHookSync(t, f.app, mention)

	n := findLatestNotification(t, f.app, f.mentionUser.Id)
	if n == nil {
		t.Fatal("expected a notification, got none")
	}
	if got := n.GetString("type"); got != "cards_mention" {
		t.Errorf("type = %q, want cards_mention (must match the description "+
			"hook's type, or one mute switch cannot cover both)", got)
	}
}

// Document packages keep sharing one switch: being @mentioned in a text or calc
// comment is one thing to a reader.
func TestCommentMention_DocumentPackagesShareOneType(t *testing.T) {
	f := seedMentionFixture(t)
	mention := mkMention(t, f.app, f, "text_comments")
	runHookSync(t, f.app, mention)

	n := findLatestNotification(t, f.app, f.mentionUser.Id)
	if n == nil {
		t.Fatal("expected a notification, got none")
	}
	if got := n.GetString("type"); got != "comment_mention" {
		t.Errorf("type = %q, want comment_mention", got)
	}
}

// --- muting ---
//
// The preference is a flat {type: bool} JSON map on a user_preferences row
// (app='notifications', key='preferences'); isNotificationMuted reads it by
// TYPE. Nothing exercised that path before, so a rename of either the type
// string or the row's app/key would have silently stopped honouring mutes.

// addUserPreferences extends the fixture with the collection isNotificationMuted
// queries. Not in the base fixture because most tests do not mute anything.
func addUserPreferences(t *testing.T, app core.App, usersID string) *core.Collection {
	t.Helper()
	c := core.NewBaseCollection("user_preferences")
	c.Fields.Add(&core.RelationField{
		Name: "user", Required: true, CollectionId: usersID, MaxSelect: 1,
	})
	c.Fields.Add(&core.TextField{Name: "app"})
	c.Fields.Add(&core.TextField{Name: "key"})
	c.Fields.Add(&core.JSONField{Name: "value"})
	if err := app.Save(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func mutePreference(t *testing.T, app core.App, userID string, prefs map[string]any) {
	t.Helper()
	usersCol, _ := app.FindCollectionByNameOrId("users")
	col := addUserPreferences(t, app, usersCol.Id)
	rec := core.NewRecord(col)
	rec.Set("user", userID)
	rec.Set("app", "notifications")
	rec.Set("key", "preferences")
	rec.Set("value", prefs)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
}

func TestCommentMention_MutedTypeIsNotDelivered(t *testing.T) {
	f := seedMentionFixture(t)
	mutePreference(t, f.app, f.mentionUser.Id, map[string]any{"comment_mention": false})

	mention := mkMention(t, f.app, f, "text_comments")
	runHookSync(t, f.app, mention)

	if got := findLatestNotification(t, f.app, f.mentionUser.Id); got != nil {
		t.Errorf("a muted mention was delivered: %v", got.Id)
	}
}

// Muting cards must NOT mute document mentions — the reason they are separate
// switches at all.
func TestCommentMention_MutingCardsLeavesDocumentMentions(t *testing.T) {
	f := seedMentionFixture(t)
	mutePreference(t, f.app, f.mentionUser.Id, map[string]any{"cards_mention": false})

	mention := mkMention(t, f.app, f, "text_comments")
	runHookSync(t, f.app, mention)

	if got := findLatestNotification(t, f.app, f.mentionUser.Id); got == nil {
		t.Error("muting cards_mention also silenced a document mention")
	}
}

// A type left enabled (or absent entirely) must still deliver — the server
// defaults to sending, so a missing key is not a mute.
func TestCommentMention_UnrelatedMuteDoesNotBlock(t *testing.T) {
	f := seedMentionFixture(t)
	mutePreference(t, f.app, f.mentionUser.Id, map[string]any{"mail_new_message": false})

	mention := mkMention(t, f.app, f, "text_comments")
	runHookSync(t, f.app, mention)

	if got := findLatestNotification(t, f.app, f.mentionUser.Id); got == nil {
		t.Error("an unrelated mute blocked a mention")
	}
}
