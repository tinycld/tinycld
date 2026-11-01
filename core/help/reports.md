---
title: Reports
summary: Make a report from your data, run it, share it, and publish it to everyone
tags: [reports, report builder, share report, publish, filter, group, pivot, formula]
order: 40
---

Open **Reports** from the rail (on a phone: **More → Reports**). The hub has three
tabs: **Mine** (reports you own), **Shared with me**, and **Published** (reports an
admin published for everyone). **Last run** shows when you last ran a report on this
device.

## Make a report

1. Select **New report** and choose what to report on.
2. In the panel beside the report, build it. The report updates as you change it.
   - **Columns**: select **Add column**. A field of a linked record (such as a
     sale's team) opens in place. Where each row has many linked records, choose
     **Add as total** (a count or a sum on each row) or **Show as child rows**.
     Rename a column, choose its **Total**, and move it up or down.
   - **Filter**: select **Add condition** to add a condition, or **Add group** for a
     nested group, and choose **All of**, **Any of** or **None of**. **Add related rows** keeps rows that have (or have
     no) linked records that match their own conditions. Turn on **Ask when the
     report runs** to let each reader set the value from a chip above the report.
   - **Group and sort**: add group levels, each with or without subtotals, and the
     order of the rows.
   - **Pivot**: spread a column with a total across one column per value, such as
     one column per month.
   - **Add formula**: build a calculated column from parts picked from menus.
   - **Layout**: for statements, show each value of the first group level as a
     section, and add computed rows such as "Income less expenses".
3. Enter a **Title** under **Details** and select **Save**.

To change a report you own or edit, open it and select **Edit report**. To keep a
copy of any report you can see, select **Save as…**.

## Run a report

- The chips above the report set its period and filters; **Add filter** adds a chip
  for a filter you can set while you read. A link copied with
  **Copy link** opens the report with the same choices.
- **Customize** changes how numbers look (thousands, cents, negatives) and which
  columns show. It never changes the figures.
- Select a group row to open or close it. **Expand all** opens every group; when any
  group is open, the same button reads **Collapse all**.
- A report that is open stays current: when someone changes the data it reads, it
  runs again.

## Share and publish

Select **Share** to add people or groups as **Viewer** (can run it) or **Editor**
(can also change and share it). Each person sees only the data they can access: a
share gives the design of the report, never your data.

An admin who owns or edits a report can turn on **Publish to everyone**. Every
member then finds it under **Published**; guests do not.

## A report marked Broken

A report is marked **Broken** when something it uses is no longer installed, for
example after a package was removed. Open it to see what is missing; its owner or an
editor can fix it with **Edit report**. A person who lost access to a report sees
"You no longer have access to this report".
