---
title: Build history & reverting
summary: Restore a previous package-install build from the Admin console
tags: [packages, install, revert, build, rollback]
order: 55
---

## What a build is

Every time you install a package from **Admin → Packages**, the app saves a
**build** — a snapshot of the server and web bundle that went live, plus a record
of the database migrations that install applied. Builds let you go back to an
earlier state if a package update misbehaves.

The first time a freshly deployed app boots, it also records a **base build** —
the image's out-of-the-box state — so you can always revert all the way back to
"before any package was installed live".

## Automatic rollback on a failed install

A build is only saved when an install **succeeds**. If a package can't even be
built — say it has a code error that fails the web bundle — the install stops
before anything goes live and your running app is **untouched**; the package shows
as **failed** in the install history.

If a package builds but then **crashes the app on restart** (for example a broken
server hook, or a database migration that fails to apply), the app notices that the
new version never came up healthy and **automatically rolls back**: it restores the
database and switches back to the build that was running before, then comes back up
on its own. That attempt is recorded as **rolled back** in the install history, and
no new build is kept. You don't need to do anything — but the package didn't
install, so check its source before trying again.

This automatic rollback is the app protecting itself on restart; the manual
**Revert** below is for deliberately going back to an earlier *working* build.

## To revert to an earlier build

1. Open **Admin → Build History**.
2. Find the build you want to return to and click **Revert**.
3. Read the confirmation carefully — it lists any **newer builds that will be
   permanently invalidated** (see below) — then confirm.

The server then:

- swaps back to that build's server binary and web bundle,
- reverses every database schema change made since that build (your **data is
  preserved** — only the schema is rolled back), and
- restarts itself. The app is briefly unavailable while it relaunches, then comes
  back on the reverted build.

## Reverting is one-way

Reverting to an older build tears down the schema added by every newer build, so
those newer builds can no longer be restored — they're marked **superseded** and
their **Revert** button disappears. To move forward again, install the newer
package version fresh; that creates a brand-new build.

For example, with builds 1 → 2 → 3 → 4 (newest), reverting to build 2 invalidates
builds 3 and 4. You cannot then jump to build 3 — install its version again instead.

## When a revert is blocked

If the database's migration history was changed outside the installer (a manual
edit, or an unrelated migration applied on top), the app can't safely compute what
to roll back and the revert is **blocked** with an explanation. Resolve the
history mismatch before trying again.

## Managing disk space

Builds are kept until you remove them — they are never auto-deleted. Each build
archives a server binary and a web bundle, which can be sizeable, so delete builds
you no longer need with the **Delete** button. The current build can't be deleted.
