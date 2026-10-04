---
title: After a failed update
summary: What to do when a database backup could not be restored after a failed update
tags: [update, rollback, backup, restore, database, automatic]
order: 57
---

## What happened

Before an update changes the database, the server makes a backup of it. If the
new version does not start, the server goes back to the version that ran before
and puts that backup back.

Sometimes the server cannot put the backup back (for example, when the disk is
full). Then the server keeps running on the database as the failed update left
it, and it keeps the backup in a separate folder:

```
<state directory>/unrestored/<build>/data.db
```

`<build>` is the build of the update that failed. The state directory is the
directory that holds `pb_data` (in the standard container, `/workspace`). The
file `unrestored.json` beside the backup tells when this happened and why the
backup could not be put back.

The server never deletes or restores this backup itself. It tells all owners
and admins once, in the app and by email.

## Automatic updates are paused

While a folder is in `unrestored/`, automatic updates do not run. **Settings →
Packages** shows "Paused: a database backup needs attention". Automatic updates
start again when the `unrestored/` folder is empty.

## To put the backup back

Do this only if no data written since the failed update must be kept. All
changes made after the backup are lost.

1. Stop the server.
2. Delete `pb_data/data.db-wal` and `pb_data/data.db-shm` if they exist. They
   belong to the current data and damage the backup if they stay.
3. Copy `unrestored/<build>/data.db` over `pb_data/data.db`. Make sure that the
   user the server runs as still owns `pb_data/data.db`.
4. Delete the folder `unrestored/<build>/`.
5. Start the server.

## To keep the current data

If the server works correctly on the current data, or if the data written
since the failed update must be kept, delete the folder
`unrestored/<build>/`. You do not have to stop the server.

## To look at the backup first

The backup is a SQLite database. To compare it with the live data, open a copy
of it with a SQLite tool (for example, `sqlite3`). Do not open the live
`pb_data/data.db` file while the server runs.

See also [Build history & reverting](help://core:build-history).
