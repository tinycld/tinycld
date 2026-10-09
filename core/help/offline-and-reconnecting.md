---
title: Offline and reconnecting
summary: What the "Offline", "Reconnecting…" and "Can't reach the server" notices at the top of the app mean, and what happens to your work meanwhile
tags: [offline, reconnecting, connection, network, sync, loading, server, retry]
order: 60
---

## The notice at the top of the app

When the app cannot reach the server for more than a moment, a small notice
appears at the top of the screen. It never covers the app: you can keep
reading what is already loaded while it shows.

- **Offline — waiting for a connection** means your device has no network
  connection. Lists show what was already loaded.
- **Reconnecting…** means your device is online but the app has lost its
  live connection to the server, or some lists cannot load yet, for example
  while the server restarts or your network changes. Lists keep trying on
  their own, and changes other people make appear again once the connection
  is back.
- **Can't reach the server. Tap for options** means your device is online
  but the server has not answered for about 20 seconds, or its health check
  failed. In the mobile app the notice names the server, for example
  "Can't reach {{server-host}}".

The notice goes away as soon as the connection is back.

## When the server cannot be reached

To see your options, tap the **Can't reach the server** notice. The window
that opens shows the address of the server the app uses, and offers:

- **Retry** checks the server now and reloads your lists. If the server
  still does not answer, the window tells you, and the app keeps trying on
  its own.
- **Choose another server** (mobile app only) takes you to the connect
  screen, where you can pick or add a different server. See
  [Servers](help://core:servers) for more. In a browser the server is the
  address of the page itself, so this option does not show.

## What happens to your work

The app keeps trying to load what you opened, and refreshes your lists when
you come back to the app or your connection returns, so you do not need to
reload the page.

Changes you make cannot reach the server while you are offline or the server
cannot be reached, and the app does not keep them to send later. While this
is so, the save or confirm button in a dialog is disabled and shows
"You're offline — changes can't be saved right now" (or the same for an
unreachable server). If a save fails anyway, the app tells you, and you can
repeat it once the notice is gone.
