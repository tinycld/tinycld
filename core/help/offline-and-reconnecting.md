---
title: Offline and reconnecting
summary: What the "Offline" and "Reconnecting…" notices at the top of the app mean, and what happens to your work meanwhile
tags: [offline, reconnecting, connection, network, sync, loading]
order: 60
---

## The notice at the top of the app

When the app cannot reach the server for more than a moment, a small notice
appears at the top of the screen. You can keep using the app while it shows.

- **Offline — waiting for a connection** means your device has no network
  connection. Lists show what was already loaded.
- **Reconnecting…** means your device is online but the server is not
  answering yet, for example while the server restarts or your network
  changes. Lists that are still loading keep trying on their own.

The notice goes away as soon as the connection is back.

## What happens to your work

The app keeps trying to load what you opened, and refreshes your lists when
you come back to the app or your connection returns, so you do not need to
reload the page.

A change you make while the notice shows cannot reach the server. If a save
fails, the app tells you, and you can repeat it once the notice is gone.

If the server stays unreachable, the app shows a full-screen message with the
server address and a way to choose a different server. See
[Servers](help://core:servers) for how to change the server you connect to.
