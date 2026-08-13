package notify

import (
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// allowedCommentCollections is the set of comment-table names the
// notify hook will dispatch for. Inserts referencing any other name
// are silently dropped — protecting against an attacker who manages
// to slip a bogus `comment_collection` value through the createRule.
// Each entry maps the comment table to its owning package's URL slug
// (e.g. `text_comments` → `text`), used to build the deep-link.
var allowedCommentCollections = map[string]string{
	"text_comments":  "text",
	"calc_comments":  "calc",
	"cards_comments": "cards",
}

// documentPackages are the packages whose content is stored as a drive
// item, and whose deep-link is therefore `/p/<slug>/<driveItemId>`.
// A package absent from this set links through linkForTarget instead.
//
// This set exists because `comment_mentions` is no longer drive-only.
// Core migration 1985000002 generalized it with `target_collection` /
// `target_record`, so a mention can now hang off any package's record —
// cards is the first — and those packages have their own URL shapes.
var documentPackages = map[string]bool{
	"text": true,
	"calc": true,
}

// RegisterCommentMentionHooks wires the OnRecordAfterCreateSuccess hook
// for comment_mentions. The hook validates the row, resolves the
// mentioned user + drive_item, then calls NotifyUser with a
// "comment_mention" payload pointing back at the doc.
func RegisterCommentMentionHooks(app *pocketbase.PocketBase) {
	registerCommentMentionHooksCore(app)
}

func registerCommentMentionHooksCore(app core.App) {
	app.OnRecordAfterCreateSuccess("comment_mentions").BindFunc(func(e *core.RecordEvent) error {
		mention := e.Record
		// Run notify off the request goroutine: external pushes can
		// stall, and a slow notify path shouldn't delay the insert
		// success response to the client.
		go handleCommentMention(app, mention)
		return e.Next()
	})
}

func handleCommentMention(app core.App, mention *core.Record) {
	commentCollection := mention.GetString("comment_collection")
	packageSlug, ok := allowedCommentCollections[commentCollection]
	if !ok {
		// Unknown comment_collection — silently drop. The allowlist
		// is the security boundary; logging at warn level so an
		// attacker probing the surface is visible without flooding
		// the log on legitimate (but unknown-to-this-build) packages.
		app.Logger().Warn("comment_mention: unknown comment_collection",
			"collection", commentCollection)
		return
	}

	// mentioned_user is a direct relation to users — drive's migration
	// 1781000000 renamed it from mentioned_user_org when the junction went
	// away. Reading the old name here returned "" and silently dropped every
	// mention notification.
	mentionedUserID := mention.GetString("mentioned_user")
	if mentionedUserID == "" {
		return
	}

	// The comment author posted the mention; if the author somehow ends up
	// as the mentioned party (e.g. a copy-paste), we already dedupe
	// self-mentions client-side in the mutations factory. Defense in depth:
	// skip here too if the comment author equals the mentioned user.
	comment, err := app.FindRecordById(commentCollection, mention.GetString("comment_record"))
	if err != nil {
		app.Logger().Warn("comment_mention: comment record not found",
			"collection", commentCollection,
			"record", mention.GetString("comment_record"),
			"error", err)
		return
	}
	if comment.GetString("author") == mentionedUserID {
		return
	}

	// Resolve what the mention points at. `target_collection` /
	// `target_record` are authoritative (core 1985000002 backfilled every
	// pre-existing row to `drive_items` + the drive_item id), but
	// `drive_item` is still read as a fallback: a row written by an older
	// client between the migration landing and that client updating would
	// carry only the legacy column.
	targetCollection := mention.GetString("target_collection")
	targetRecord := mention.GetString("target_record")
	driveItemID := mention.GetString("drive_item")
	if targetRecord == "" {
		targetCollection, targetRecord = "drive_items", driveItemID
	}
	if targetRecord == "" {
		return
	}

	threadID := commentThreadID(comment)
	suggestionID := comment.GetString("suggestion_id")
	url := linkForTarget(app, packageSlug, targetRecord, threadID, suggestionID)

	authorName := comment.GetString("author_name")
	if authorName == "" {
		authorName = "Someone"
	}

	title := fmt.Sprintf("%s mentioned you", authorName)
	body := truncate(comment.GetString("body"), 200)

	NotifyUser(app, NotifyParams{
		UserID:  mentionedUserID,
		Type:    mentionTypeFor(packageSlug),
		Package: packageSlug,
		Title:   title,
		Body:    body,
		URL:     url,
		Meta: map[string]any{
			"commentCollection": commentCollection,
			"commentRecord":     comment.Id,
			// driveItem is kept for drive-backed packages so existing
			// consumers of the payload keep reading the field they expect;
			// it is empty for a non-document package.
			"driveItem":        driveItemID,
			"targetCollection": targetCollection,
			"targetRecord":     targetRecord,
			"threadId":         threadID,
			"suggestionId":     suggestionID,
		},
	})
}

// mentionTypeFor picks the notification TYPE, which is what the per-user mute
// preference is keyed on (see isNotificationMuted + core's
// lib/use-notification-preferences.ts).
//
// Document packages share `comment_mention`: being @mentioned in a text or calc
// comment is one thing to a reader, and one switch should silence it.
//
// A non-document package gets `<slug>_mention` instead, because its mentions
// arrive at a different cadence and are worth muting separately — a board is
// chattier than a document. This ALSO keeps a package's two mention sources on
// one switch: cards' description mentions come from its own flush hook
// (cards/server/description_mentions.go) and already send `cards_mention`, so a
// comment mention must too, or muting cards would silence only half of them.
func mentionTypeFor(packageSlug string) string {
	if documentPackages[packageSlug] {
		return "comment_mention"
	}
	return packageSlug + "_mention"
}

// linkForTarget builds the deep-link back to the mentioned content.
//
// Document packages (text, calc) store content as a drive item and route it
// at `/p/<slug>/<driveItemId>`. For an anchored comment the `?thread=<id>`
// param is read by the document screen's useCommentsLifecycle hook to open the
// drawer focused on the mentioned thread; a suggestion-reply mention (non-empty
// `suggestion_id`) uses `?focusSuggestion=<id>` instead, so the review drawer
// opens on the matching suggestion row rather than the comments drawer.
//
// Every other package owns its own route, so there is no drive item to point
// at and no generic shape to guess. Cards routes its board at `/cards` and
// opens a card with `?focused=<key|id>` (usePeekUrl). The record id is used
// rather than the OTTER-123 key because the key is composed from the board's
// slug and a server-assigned number that this hook would have to re-derive —
// resolveOnBoard accepts either spelling, so the id is the link that cannot go
// stale.
//
// Routes are slug-scoped — single-org, so no org slug in the path.
func linkForTarget(app core.App, packageSlug, targetRecord, threadID, suggestionID string) string {
	appURL := strings.TrimRight(app.Settings().Meta.AppURL, "/")

	if documentPackages[packageSlug] {
		if suggestionID != "" {
			return fmt.Sprintf("%s/p/%s/%s?focusSuggestion=%s",
				appURL, packageSlug, targetRecord, suggestionID)
		}
		return fmt.Sprintf("%s/p/%s/%s?thread=%s",
			appURL, packageSlug, targetRecord, threadID)
	}

	if packageSlug == "cards" {
		return fmt.Sprintf("%s/cards?focused=%s", appURL, targetRecord)
	}

	// A package in the allowlist but with no link shape declared. Better a
	// notification that lands in the bell with a bare package link than one
	// that is dropped entirely.
	return fmt.Sprintf("%s/%s", appURL, packageSlug)
}

// commentThreadID returns the root comment id for the thread. For
// replies, parent_comment points at the root; for the root itself,
// parent_comment is empty and the row's own id is the thread id.
func commentThreadID(comment *core.Record) string {
	parent := comment.GetString("parent_comment")
	if parent != "" {
		return parent
	}
	return comment.Id
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
