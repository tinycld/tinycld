package sharelink

import (
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/driveshare"
	"tinycld.org/core/realtime"
)

// Realtime-room admission for share-link visitors — the one definition shared
// by every package that opens a realtime room over a drive_item (text and calc
// today). Formerly duplicated byte-for-byte in both packages' servers.

// AuthorizeAnonRoom admits an anonymous share-link visitor to a realtime room
// bound to a drive_item. Re-resolves the share link (rejecting revoked/
// expired/downgraded links at connect time) and admits any recognized share
// role (viewer, commentor, or editor) bound to this exact drive_item.
// Non-editor roles are admitted read-only: write enforcement is delegated to
// the broker's WritePredicate (ReadOnlyForConn / SetReadOnly in OnConnect).
//
// claimedItemID is the item id the visitor's session claims; it must match the
// room being joined before the link is even resolved.
func AuthorizeAnonRoom(app core.App, shareToken, claimedItemID, roomID string) error {
	if claimedItemID != roomID {
		return driveshare.ErrNoAccess
	}
	link, item, err := ResolveLink(app, shareToken)
	if err != nil {
		return err
	}
	if item.Id != roomID {
		return driveshare.ErrNoAccess
	}
	switch link.GetString("role") {
	case RoleViewer, RoleCommentor, RoleEditor:
		// Admit — read-only enforcement for non-editor roles happens via
		// the broker WritePredicate (ReadOnlyForConn), so viewers and
		// commentors may open the room but cannot write.
	default:
		return driveshare.ErrNoAccess
	}
	return nil
}

// ReadOnlyForConn reports whether the connecting client lacks edit rights on
// the room's drive_item.
//
// The result is used for BOTH the client serverHello signal (which disables
// the editor UI) and the cached flag the broker's WritePredicate enforces. So
// a viewer that ignores readOnly=true is still rejected server-side — its
// MsgDocUpdate frames are dropped by the broker. Compute once at connect
// (OnConnect → SetReadOnly); it is not re-queried per frame.
func ReadOnlyForConn(app core.App, roomID string, conn *realtime.Client) bool {
	// Anonymous share-session visitor: the editable route only admits
	// editor-role links (enforced in AuthorizeShare), so an anon
	// connection here is writable. They have no drive_shares row, so
	// don't run the member resolution below.
	if conn.IsAnonymous() {
		return conn.ShareRole() != RoleEditor
	}
	userID := conn.AuthID()
	if userID == "" {
		return true
	}
	return !driveshare.CanWrite(app, userID, roomID)
}
