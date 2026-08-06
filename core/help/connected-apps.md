---
title: Connected apps and devices
summary: Connect the command line tool or a third-party integration, and revoke access you no longer want.
tags: [security, cli, integrations]
order: 40
---

To see what has access to your account, open **Settings → Personal** and find
**Connected apps**. Each row is one device or integration, with when it was
last used.

## Connecting the command line tool

Run the login command on your computer:

```
tinycld auth login {{server-host}}
```

It shows a short code and opens {{server-host}} in your browser. Check that the
code in the browser matches the one in your terminal, name the device so you
can recognize it later, and choose **Connect**.

If the browser does not open, go to `{{server-host}}/p/oauth/authorize` and
enter the code by hand.

## Revoking access

Choose **Revoke** next to any entry. The change takes effect immediately — the
next request from that device or integration is refused, and it must be
connected again from scratch.

Revoking one entry does not affect anything else. Your browser session, your
phone, and every other connected device keep working.

If you are an admin and want to cut off an entire integration for everyone at
once, rather than one device at a time, see
[Managing OAuth clients](help://core:oauth-clients).

## What an app can do

When you connect something, the approval screen lists exactly what it is asking
for — reading email, creating files, and so on. An app only ever gets what is
listed there. If a request asks for more than you expect, choose **Deny**.
