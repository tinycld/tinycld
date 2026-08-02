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
- **admin** — manage the organization: settings, members and invites, and the
  Admin console (apart from Packages), in addition to everyday use.
- **member** — everyday use of the installed packages: create and edit your own
  content and anything shared with you.
- **guest** — limited access granted through shared links and invites. Guests
  can view (and comment, where invited) but can't create workspace content.

Owners and admins set roles under **Settings → Members**. Only an owner can
make someone else an owner.

## The Admin console

Owners and admins get a shield icon in the nav rail. It opens the **Admin
console**, the deployment-wide area for organizations, build history and
system settings — concerns that affect the whole deployment rather than one
person, which is why they live outside the normal settings.

**Packages** — installing, removing and upgrading them — appears there for
**owners only**. A package change rebuilds what everyone on the deployment
runs, so it isn't an ordinary administrative action.

If you don't see the shield icon, your role is member or guest.
