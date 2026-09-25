---
title: Backups & restore
summary: Back up the whole organization to your own storage, and restore it
tags: [backup, restore, export, disaster recovery, s3]
order: 56
---

## What a backup contains

A backup is one encrypted file. It holds the database, every uploaded file,
and the list of installed packages with their versions. It does not hold
build output — a restore rebuilds the packages from that list.

## Back up from Settings

1. Open **Settings → Backups**.
2. Enter an **upload URL**. The server streams the backup to this URL with an
   HTTP PUT, so use a presigned PUT URL from your storage provider (S3, R2,
   B2, GCS and MinIO all support these). Presign it **without** a
   content-length condition; the upload is chunked.
3. Enter a **passphrase** of at least 12 characters, twice. The backup is
   encrypted with it. If you lose the passphrase, the backup cannot be read
   by anyone, including us.
4. Choose **Start backup**. The row in **History** shows progress and the
   result. Owners and admins receive a notification when it finishes.

To make a presigned PUT URL with the AWS CLI:

```
aws s3 presign --expires-in 3600 s3://my-bucket/tinycld/backup.age --method PUT
```

## With the command line

`tinycld backup create --out ./backup.age` streams the backup to a file on
your computer. See [Command line](help://core:command-line) to install and
sign in. The CLI prompts for the passphrase.

## Restore

Only an owner can restore. Restoring **replaces all current data** with the
backup. Before it does, the server makes a safety copy of the current data
and shows you its key in **History** until the restore succeeds.

1. Open **Settings → Backups → Restore**.
2. Enter a **download URL** for the backup (a presigned GET URL) and the
   passphrase.
3. Confirm that current data is replaced and choose **Restore**.

The app is unavailable while the restore runs: it rebuilds the packages the
backup lists, then restarts with the restored data. If the download URL
expires during a long restore, the History row asks for a fresh URL and
continues where it stopped.

With the command line: `tinycld backup restore --from ./backup.age`.

## Self-hosted servers

A Docker self-host restores exactly like a hosted organization. A single
binary cannot rebuild packages: it restores when the backup's package set
matches the binary's, and refuses otherwise. `tinycld backup restore --force`
restores the data anyway and lets the binary apply any pending migrations;
packages the binary lacks stay unavailable.

Settings managed outside the organization (error reporting, web push, mail
sending on a hosted instance) are not part of a backup. Enter them again
after restoring onto a different server.

## Limits

One backup or restore runs at a time, and never during a package install.
Manual backups are limited to a daily number per organization.

The server refuses a backup or a restore that does not fit on the disk, and
tells you how much space it needs. A restore needs room for the backup file,
a safety copy of the current data, and the current data itself.

The safety copy from the most recent failed restore is kept until the next
restore starts. Copies from earlier failed restores are removed then.
