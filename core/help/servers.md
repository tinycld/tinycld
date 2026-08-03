---
title: Servers on your device
summary: Adding more than one server to the app and switching between them
tags: [servers, switch, connect, mobile, sign in]
order: 22
---

## One app, several servers

The app can hold several servers at once — a work server and a home one, or two
organizations you belong to. Each keeps its **own account, its own data, and its
own sign-in**. Switching between them doesn't sign you out of any of the others.

This is the app on your phone or tablet. In a web browser the address bar already
decides which server you're on, so there's nothing to switch — see
[Organizations](help://core:organizations) for how that works.

## Adding a server

Open the **More** menu (the ⋯ tab at the bottom on a phone) and choose **Add
server** under **Servers**. Type the address and tap **Connect**.

You can type just the hostname — `acme.tinycld.org` is enough, and the app fills
in the rest. If you host your own server, use the full address you'd type in a
browser.

Adding a server doesn't disturb the one you're already on. You'll sign in to the
new server, and both stay signed in from then on.

## Switching

The **Servers** section of the **More** menu lists every server you've added,
with a check beside the one you're on. Tap another to switch to it. On a tablet
the same list is in the user menu, at the bottom of the nav rail.

**The app restarts when you switch.** That's expected and takes a second or two:
each server can run a different set of packages, so the app reloads to match the
one you're moving to. You won't be asked to sign in again — that server remembers
you.

Switching back is just as quick. Nothing is re-downloaded and nothing is
re-entered.

## Removing a server

Go to **Settings → This device → Servers** and use the trash icon beside a
server. Removing signs you out of that server on this device and forgets its
address; your account on the server itself is untouched, and you can add it again
later.

If you remove the server you're currently using, the app switches to another one
you've saved. Remove the last one and you'll be asked to connect somewhere.

Removing is deliberately kept out of the quick-switch menu so it isn't a mis-tap
away from the server you meant to switch to.
