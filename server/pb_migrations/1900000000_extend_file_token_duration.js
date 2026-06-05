/// <reference path="../../../server/pb_data/types.d.ts" />
// Raise the users-collection file-token lifetime from PocketBase's 180s (3min)
// default to 1h. File tokens authorize `?token=` reads of protected files
// (thumbnails, previews, downloads). At 3min the token — and therefore the
// file URL — rotated every few minutes, busting the browser's URL-keyed HTTP
// cache so thumbnails re-downloaded constantly. A 1h token keeps the URL stable
// long enough for the browser cache to actually hit. The client refreshes the
// cached token at 55min (see core's use-authed-file-url), comfortably ahead of
// expiry. Trade-off: a leaked file URL stays readable for up to 1h instead of
// 3min — acceptable for file reads.
const FILE_TOKEN_DURATION = 3600 // 1 hour, in seconds
const PB_DEFAULT_FILE_TOKEN_DURATION = 180 // 3 minutes

migrate(
    app => {
        const users = app.findCollectionByNameOrId('users')
        users.fileToken.duration = FILE_TOKEN_DURATION
        app.save(users)
    },
    app => {
        const users = app.findCollectionByNameOrId('users')
        users.fileToken.duration = PB_DEFAULT_FILE_TOKEN_DURATION
        app.save(users)
    }
)
