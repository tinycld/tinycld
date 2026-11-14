---
title: Set up AI classification
summary: Turn on AI labelling with an OpenRouter key, set a monthly budget, and see what is sent.
tags: [ai, classification, admin, openrouter]
order: 60
---

To turn on AI classification, an admin opens **Settings → AI classification**, enters an OpenRouter API key, selects **Save**, and then selects **Verify connection**. With no key, the feature is off for everyone.

To turn it off again, select **Remove key**. **Verify connection** tests the saved key, so save a new key before you verify it.

## Budget

Set **Monthly budget (USD)**. Before each request the server checks that the month's spend plus the request's estimated cost stays inside the budget. When the budget is reached, items are not classified until the next month. Also set a spending limit on the key in OpenRouter.

## Thresholds

- **Read the full text below this confidence** (default 0.7): when the first answer, from metadata only, is less sure than this, the server also sends the item's text and asks again.
- **Automatic action threshold** (default 0.9, from 0.5 to 1): apps take their automatic action, such as archiving mail, only at or above this confidence. Only admins can change it.

## When classification pauses

If OpenRouter rejects the key, reports no credits, or denies the model, classification pauses and a banner shows the reason. Fix the problem in OpenRouter, then select **Verify connection** to resume.

## What is sent

For each item, the server sends the label names and their rules, plus metadata such as sender, recipients, subject and a short snippet. When the first answer is not confident enough, it also sends the item's text. Attachments are never sent. Each item is a separate request. Read OpenRouter's privacy and logging policy before you process sensitive data.
