---
title: Automation rules
summary: Automate repetitive steps when things happen
tags: [rules, automation, workflow]
order: 40
---

## What a rule does

A rule watches for something happening in your workspace and reacts to it
automatically. Every rule has the same shape:

- **When** — the trigger that starts the rule (a schedule, a manual run, or an
  event from a package like mail).
- **If** *(optional)* — conditions that narrow which occurrences count.
- **Then** — one or more actions to run, such as sending you a notification,
  applying a label, or updating a record.

## Creating a rule

Open **Settings → Rules → New rule**. The builder walks through the three
parts in order:

1. **When** — pick a trigger from the menu, grouped by package. Choosing a
   trigger resets any conditions and actions below it, since they're specific
   to that trigger's fields.
2. **If** — appears only for triggers tied to a record (not for "Run
   manually" or "On a schedule", which have no fields to filter on). Add one
   or more conditions, and combine multiple conditions or groups with "all"
   (AND) or "any" (OR).
3. **Then** — add one or more actions. Some actions take parameters — for
   text parameters tied to the trigger's fields, the `{{ }}` button inserts a
   placeholder like `{{subject}}` that's filled in with the actual value when
   the rule runs.

Select **Save** when you're done. If something's missing — no name, no
trigger, no action — the builder lists exactly what to fix before it will
save.

## Personal vs. organization rules

Settings → Rules has two segments:

- **My rules** — rules only you see and edit. Anyone can create personal
  rules.
- **Organization** — shared rules visible to everyone, but only **admins and
  the owner** can create, edit, or delete them. Other members see organization
  rules listed but read-only.

Organization rules run with **admin authority**: when an org rule's actions
touch a record, they act as if an admin performed them, regardless of who (or
what event) triggered the run. This matters for triggers on shared resources
— a shared mailbox, for example — where an individual member might not
otherwise have access.

## Testing a rule

Before relying on a rule, you can test it two ways:

- **Dry run** — in the builder, select **Test against recent items** to see
  how many of your most recent matching items the rule's conditions would
  catch, without actually running any actions. This is available for triggers
  tied to a record.

  Some triggers — like ones on a shared mailbox — can't always be scoped to
  "recent items you can see." If you get a message saying the test can't run
  that way, it means an **organization admin** needs to test it instead (from
  the Organization segment), since dry runs on those triggers require admin
  visibility into the underlying data.

- **Run now** — for "Run manually" and "On a schedule" rules, the row's
  overflow menu has a **Run now** action that fires the rule immediately,
  regardless of whether it's enabled. Use this to confirm the actions
  actually do what you expect — for example, running a manual notify rule and
  checking the notification bell.

## Run history

Every time a rule fires, it logs a run. Open a rule's overflow menu and
select **Run history** to see:

- Whether the run **matched** — its conditions were satisfied and its actions
  ran — or shows **Didn't match**, meaning the trigger fired but the
  conditions filtered it out. "Didn't match" rows are expected and useful:
  they confirm the rule is watching, even when there's nothing to do.
- How long the run took, and the result of each action (success or error).

## Auto-disable

If a rule's actions keep failing across multiple runs, the rule is
automatically disabled so it stops generating repeated errors. Check its run
history for the failure details, fix the underlying issue (for example, a
deleted label or missing permission), then re-enable it from the rules list.

[Mail rules](help://mail:rules) covers the mail package's built-in triggers
and actions in more detail.
