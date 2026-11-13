---
title: Set up AI classification
summary: Turn on AI labelling with an OpenRouter key, set a monthly budget, and see what is sent.
tags: [ai, classification, admin, openrouter]
order: 60
---

To turn on AI classification, an admin opens **Settings → AI classification**, enters an OpenRouter API key, and selects **Verify connection**. With no key, the feature is off for everyone.

## Budget

Set **Monthly budget (USD)**. Before each request the server checks that the month's spend plus the request's estimated cost stays inside the budget. When the budget is reached, items are not classified until the next month. Also set a spending limit on the key in OpenRouter.

## When classification pauses

If OpenRouter rejects the key, reports no credits, or denies the model, classification pauses and a banner shows the reason. Fix the problem in OpenRouter, then select **Verify connection** to resume.

## What is sent

For each item, the server sends the label names and their rules, plus metadata such as sender, recipients, subject and a short snippet. When the first answer is not confident enough, it also sends the item's text. Attachments are never sent. Each item is a separate request. Read OpenRouter's privacy and logging policy before you process sensitive data.
