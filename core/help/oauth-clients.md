---
title: Managing OAuth clients
summary: Review the applications allowed to request access to your organization, and turn one off if you no longer trust it.
tags: [security, integrations, admin]
order: 41
---

An **OAuth client** is an application permitted to ask your users for access —
the command line tool, and any integration you add later. This is different
from [connected apps](help://core:connected-apps), which is where each person
manages their own devices. A client is org-wide: it applies to everyone.

Only admins and owners can see or change this. Open **Settings → OAuth
clients**.

## What each row tells you

Each row is one registered client, with its identifier, how many scopes it may
request, and how many connections are live through it right now. That
connection count is what you lose if you turn it off.

A client marked **first-party** ships with TinyCld. The command line tool is
registered this way so it works the moment your server starts.

## Turning a client off

Switch a client off when you no longer trust it — a compromised integration, or
one you have stopped using. This takes effect immediately and does two things:

- Nobody can connect it again, or sign in through it.
- Every connection it already holds stops working on the very next request.

That second part is the important one. Revoking a single device from
**Connected apps** stops one connection; turning the client off stops all of
them at once, for every user, without needing to find them one by one.

## Turning it back on

Nothing is deleted. Existing connections are suspended, not destroyed, so
switching the client back on restores them — your users do not have to
reconnect. This makes it safe to turn a client off the moment you are
suspicious and investigate afterwards.

If you want a client's access gone permanently rather than suspended, remove
the client itself from the PocketBase admin. That deletes its connections for
good and cannot be undone.
