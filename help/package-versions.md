---
title: Updating & downgrading packages
summary: Change installed package versions from Setup, one at a time or in a group
tags: [packages, versions, update, upgrade, downgrade, compatibility]
order: 56
---

## What this does

**Setup → Versions** lists every installed package with its current version and
whether a newer version is available. You can move any package to a different
version — **newer (update)** or **older (downgrade)** — and apply several changes
together. Unlike a build revert, a version change touches only the selected
package's data and schema; every other package is left exactly as it was.

Available versions come from wherever the package was installed:

- **npm packages** — the versions published to the registry.
- **git packages** — the repository's release tags.

A package whose source can't be reached shows _"Couldn't check versions"_ and is
left unselectable until the source is reachable again.

## To update one or more packages

1. Open **Setup → Versions**.
2. For each package you want to change, pick a target version from its dropdown.
   Choosing a version different from the current one adds it to the pending set.
3. Use **Select all updates** to queue every package that has a newer version at
   once, or **Clear** to drop your selection.
4. The selection is checked for compatibility as you go (see below). When it's
   clean, press **Apply N changes**.
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
require. When your selection would leave one of those requirements unsatisfied,
the **Incompatible selection** banner lists each conflict (which package requires
what, and what version it found), and **Apply** stays disabled until you resolve
it — usually by adding the required package to the same change set, or picking a
different target version.

The same check runs again on the server immediately before anything is applied,
so a selection can never slip through in a stale state.

## Versions vs. build history

[Build history](help://core:build-history) reverts the **whole image** to an
earlier snapshot, undoing every package change made since — use it for recovery.
The **Versions** tab changes **one package at a time** and leaves the rest
untouched — use it for routine updates and targeted downgrades.
