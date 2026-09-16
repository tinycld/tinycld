---
title: Settings your provider manages
summary: Why some system settings are missing when someone else hosts this deployment
tags: [settings, hosting, provider, system]
order: 400
---

## What you are seeing

Some settings screens are not here. Under **Settings → System** you may expect
entries for error reporting, web push, or the mail provider and find that one or
more of them is absent, or opens a note saying it is configured by your hosting
provider.

That is deliberate, and it means this deployment is run by a hosting provider
rather than by you.

## Why they are not yours to set

These three settings are not preferences — they are accounts and credentials
with a third-party service:

- **Mail provider** — the Postmark account or SMTP host all outgoing mail flows
  through.
- **Web push** — the signing keypair browsers check when your deployment sends a
  notification.
- **Error reporting** — the Sentry project crash reports are filed to.

A hosting provider runs one of each for every deployment they host. They hold
the credentials, they pay for the accounts, and they are the ones who can rotate
them. Those values never reach this deployment's own database, so a form here
could not save them — it would appear to work and change nothing.

## What still works

Everything that is genuinely yours stays yours:

- Mail **domains** — adding your own domain, and its DNS verification — are
  managed here as usual, under **Settings → Mail → Domains**.
- Mailboxes, labels, rules, and every per-person preference are unchanged.
- Your organization's name, logo, and members are yours to manage.

Mail still sends, push notifications still arrive, and errors are still
reported. The configuration behind them simply belongs to whoever runs this
deployment for you.

## If you need something changed

Ask your hosting provider. A change to any of these applies to every deployment
they host, so it is theirs to make — and it takes effect here without this
deployment restarting.

## Running TinyCld yourself

If you install TinyCld on your own server, none of this applies: every one of
these screens is present and every value is yours to set. See
[Installing packages](help://core:installing-packages) for what a self-run
deployment administers.
