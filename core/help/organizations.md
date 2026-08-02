---
title: Organizations
summary: How your organization's workspace works and what member roles mean
tags: [org, workspace, roles]
order: 20
---

## What is an organization?

An **organization** (org) is a workspace that holds its own contacts, mail,
files, and everything else. Each organization runs at its own web address — the
address in your browser is the organization, and everything you see there
belongs to it.

If you belong to more than one organization — say a personal one and a work
one — each has its own address and its own sign-in. Once you've signed into an
organization in this browser, it appears in the **Organizations** section of
the user menu (and the More drawer on mobile), so you can jump between them
without retyping addresses. Switching opens the other organization at its own
address; you stay signed in there independently.

## Roles

Every member has one role on their account:

- **owner** — everything an admin can do, plus the two things reserved to the
  owner: installing, removing and upgrading packages, and granting the owner
  role to someone else.
- **admin** — manage the organization: settings, members and invites, storage
  and the audit log, in addition to everyday use.
- **member** — everyday use of the installed packages: create and edit your own
  content and anything shared with you.
- **guest** — limited access granted through shared links and invites. Guests
  can view (and comment, where invited) but can't create workspace content.

Owners and admins set roles under **Settings → Members**. Only an owner can
make someone else an owner.

## Administering the organization

Everything administrative lives in **Settings**, reached from the gear icon in
the nav rail. Owners and admins see an **Organization** group there — storage,
members, labels, packages and the audit log — that members and guests don't.

Two of those are **owner-only**, because they change what everyone on the
deployment runs rather than how one organization is configured:

- the **Install & Versions** section at the bottom of **Settings → Packages**,
  for installing, removing and upgrading packages
- **Settings → Build History**, for reverting to an earlier build

Admins still manage the rest of **Settings → Packages** — turning installed
packages on and off for this organization.

If you don't see the Organization group at all, your role is member or guest.
