---
title: Updating & downgrading packages
summary: Change installed package versions from Admin → Packages, one at a time or in a group
tags: [packages, versions, update, upgrade, downgrade, compatibility]
order: 56
---

## What this does

**Admin → Packages** lists every installed package. Each row shows the package's
current version and, when a different version is available, a version picker right
on the row. You can move any package to a different version — **newer (update)**
or **older (downgrade)** — and apply several changes together. Unlike a build
revert, a version change touches only the selected package's data and schema;
every other package is left exactly as it was.

Available versions come from wherever the package was installed:

- **npm packages** — the versions published to the registry.
- **git packages** — the repository's release tags.

A package whose source can't be reached shows _"Couldn't check versions"_, and a
package with only its current version (or a bundled/built-in one) shows its
version as plain text with no picker — there's nothing to move it to.

## To update one or more packages

1. Open **Admin → Packages**. Rows with a newer version available show an
   **Update** badge.
2. For each package you want to change, pick a target version from its row's
   picker. Choosing a version different from the current one stages it — the row
   highlights and shows an **upgrade ▲** or **downgrade ▼** flag, and its
   enable/disable, edit, drag, and uninstall controls lock until you apply or
   clear the change.
3. Use **Stage all updates** to queue every package that has a newer version at
   once, or **Clear** to drop your staged changes.
4. The staged set is checked for compatibility as you go (see below). When it's
   clean, the footer reads **Compatible · N staged**; press **Apply N changes**.
5. A progress window streams each step. When it finishes the app restarts briefly
   and comes back on the new versions.

## Downgrading drops data

Moving a package to an **older** version reverses the database migrations that
the newer version added — which can **drop columns or whole collections** and the
data in them. Because this is destructive:

- The confirmation dialog lists exactly which collections and fields the
  downgrade will drop.
- You must type the package's slug to confirm.
- The database is backed up automatically before the change, so a failed
  downgrade rolls back cleanly.

Only downgrade when you're sure you no longer need the data the newer version
introduced.

## Compatibility checking

Packages can declare which versions of `@tinycld/core` and of other packages they
require. When your staged set would leave one of those requirements unsatisfied,
the conflict panel below the list spells out each one (which package requires
what, and what version it found), the footer shows **N conflicts**, and **Apply**
stays disabled until you resolve it — usually by staging the required package in
the same change set, or picking a different target version.

The same check runs again on the server immediately before anything is applied,
so a staged set can never slip through in a stale state.

## Versions vs. build history

[Build history](help://core:build-history) reverts the **whole image** to an
earlier snapshot, undoing every package change made since — use it for recovery.
A version change on **Admin → Packages** moves **one package at a time** and
leaves the rest untouched — use it for routine updates and targeted downgrades.

## Installing an exact version

The row picker only offers versions the app discovered from the package's source.
To install a specific version it didn't list — or a pinned git ref — use
**Install package** and give the full spec (for example `@tinycld/contacts@2.3.1`
or `github:acme/pkg#v1.2.0`). See
[Installing packages](help://core:installing-packages).
