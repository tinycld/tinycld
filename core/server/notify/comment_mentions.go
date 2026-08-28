package notify

import (
	"fmt"
	"strings"

	"tinycld.org/core/approutes"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// commentCollectionSuffix is the naming convention every comment table
// follows: `<slug>_comments`. The owning package's URL slug is the prefix.
const commentCollectionSuffix = "_comments"

// packageSlugForCommentCollection derives the owning package's slug from a
// comment table name, or "" when the name does not follow the convention.
//
// Core deliberately keeps NO list of which packages may have comments. A
// third-party package must be able to add commenting without core shipping a
// release to permit it, and an allowlist here would be a list of first-party
// packages baked into core — the coupling the lean-shell guarantee forbids.
//
// This is not the authorization boundary and never was. `comment_mentions`
// rows are gated by the collection's createRule (core migration 1985000003
// creates it as superuser-only; each package appends a branch that resolves
// the caller's access through the target record). A row reaching this hook has
// already passed that check; all this decides is which URL to point at.
func packageSlugForCommentCollection(commentCollection string) string {
	if !strings.HasSuffix(commentCollection, commentCollectionSuffix) {
		return ""
	}
	return strings.TrimSuffix(commentCollection, commentCollectionSuffix)
}

// isDocumentTarget reports whether the mention hangs off a drive item — the
// storage shape document packages use. Read from the row's target_collection
// rather than a list of package slugs, so a third-party document package gets
// the same treatment without core naming it.
func isDocumentTarget(targetCollection string) bool {
	return targetCollection == "drive_items"
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
	packageSlug := packageSlugForCommentCollection(commentCollection)
	if packageSlug == "" {
		// Not a `<slug>_comments` name, so there is no slug to build a link
		// from. Authorization already happened at the collection's createRule;
		// this is a naming mismatch, not a rejection.
		//
		// Info, not warn: comment_collection is free text any authenticated
		// commenter can set, so paging on it would hand every account holder a
		// remote pager DoS. Info still reaches stderr and _logs.
		log.Info("comment mention: comment_collection does not follow <slug>_comments",
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
		// Info, not warn: the comment being gone by the time this async
		// handler runs is a normal delete race, not an operator problem.
		log.Info("comment mention: comment record not found",
			"collection", commentCollection,
			"recordID", mention.GetString("comment_record"),
			"err", err)
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
	url := linkForTarget(app, packageSlug, targetCollection, targetRecord, threadID, suggestionID)

	authorName := comment.GetString("author_name")
	if authorName == "" {
		authorName = "Someone"
	}

	title := fmt.Sprintf("%s mentioned you", authorName)
	body := truncate(comment.GetString("body"), 200)

	NotifyUser(app, NotifyParams{
		UserID:  mentionedUserID,
		Type:    mentionTypeFor(packageSlug, targetCollection),
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
func mentionTypeFor(packageSlug, targetCollection string) string {
	if isDocumentTarget(targetCollection) {
		return "comment_mention"
	}
	return packageSlug + "_mention"
}

// linkForTarget builds the deep-link back to the mentioned content.
//
// A drive-item target routes at `/p/<slug>/<driveItemId>` — the public document
// tree, shared by any package that stores content as a drive item. For an anchored comment the `?thread=<id>`
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
func linkForTarget(
	app core.App,
	packageSlug, targetCollection, targetRecord, threadID, suggestionID string,
) string {
	appURL := strings.TrimRight(app.Settings().Meta.AppURL, "/")

	if isDocumentTarget(targetCollection) {
		if suggestionID != "" {
			return fmt.Sprintf("%s/p/%s/%s?focusSuggestion=%s",
				appURL, packageSlug, targetRecord, suggestionID)
		}
		return fmt.Sprintf("%s/p/%s/%s?thread=%s",
			appURL, packageSlug, targetRecord, threadID)
	}

	// Everything else: the package root with ?focused=<record>. A convention a
	// package opts into (cards' usePeekUrl is the reference implementation) and
	// may ignore, in which case the reader still lands somewhere useful.
	// Stated as a convention because the previous shape switched on the slug,
	// so a third-party package could not get a deep link without core naming it.
	return fmt.Sprintf("%s%s?focused=%s", appURL, approutes.Href(packageSlug), targetRecord)
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
