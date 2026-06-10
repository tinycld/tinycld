---
title: Super admins & the Admin console
summary: Who can reach the cross-org Admin console and how to grant access
tags: [admin, super-admin, packages, access]
order: 15
---

## What a super admin is

A **super admin** can open the **Admin console** — the deployment-wide area for
managing packages, organizations, package versions, and build history. These are
cross-org concerns (they affect the whole deployment, not a single org), so they
live outside the normal org settings.

Super-admin access is a separate grant from any org role. Being an **owner** or
**admin** of an organization does **not** make you a super admin, and a super
admin isn't automatically a member of every org.

## Opening the Admin console

If you're a super admin, a shield icon appears in the nav rail (near Settings).
Click it to open the console using your normal session — there's no second login.

If you don't see the icon, you haven't been granted super-admin access.

## Granting and revoking access

Inside the Admin console, open **Super Admins**:

- **Grant access** — enter the email of an existing user to make them a super
  admin. They'll see the Admin icon on their next load, no re-login needed.
- **Revoke** — remove a user's super-admin access with the trash icon on their
  row. Their Admin icon disappears.

Any super admin can grant or revoke another. On a brand-new deployment, the first
super admin is granted by the PocketBase superuser who completed initial setup —
they reach the console through the superuser login, then add the first app user
here.
