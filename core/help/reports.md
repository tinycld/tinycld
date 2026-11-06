---
title: Reports
summary: Make a report from your data, run it, share it, and publish it to everyone
tags: [reports, report builder, share report, publish, filter, group, pivot, formula, totals, groups, lookup, formulas, phone]
order: 40
---

Open **Reports** from the rail (on a phone: **More → Reports**). The hub has three
tabs: **Mine** (reports you own), **Shared with me**, and **Published** (reports an
admin published for everyone). **Last run** shows when you last ran a report on this
device.

A package can open the hub for its own reports (for example from its sidebar). The
hub then shows "Showing reports from" and the package's name, and one list of every
report of that package you can open: your own, the ones shared with you, and the
published ones. To see all reports in the tabs again, select **Show all reports**.

## Make a report

1. Select **New report** and choose what to report on.
2. In the panel beside the report, build it. The report updates as you change it.
   - **Columns**: select **Add column**. A field of a linked record (such as a
     sale's team) opens in place. Where each row has many linked records, choose
     **Add as total** (a count or a sum on each row) or **Show as child rows**.
     Rename a column, choose its **Total**, and move it up or down.
   - **Filter**: select **Add condition** to add a condition, or **Add group** for a
     nested group, and choose **All of**, **Any of** or **None of**. **Add related
     rows** keeps rows that have (or have no) linked records that match their own
     conditions. Turn on **Ask when the report runs** to let each reader set the
     value from a chip above the report.
   - **Group and sort**: add group levels, each with or without subtotals, and the
     order of the rows.
   - **Pivot**: spread a column with a total across one column per value, such as
     one column per month, or one column per linked record, such as one column per
     team. Under **Show as**, choose the record's field that names its columns.
   - **Add formula**: build a calculated column from parts picked from menus.
   - **Layout**: for statements, show each value of the first group level as a
     section, and add computed rows such as "Income less expenses".
3. Enter a **Title** under **Details** and select **Save**.

To change a report you own or edit, open it and select **Edit report**. To keep a
copy of any report you can see, select **Save as…**.

## Totals

To total only what a report shows, open the column in **Columns** and set **Total** to **Add the rows shown**. This choice is only for number and amount columns. **Recompute from all rows** (the default) totals every row the filter keeps.

With **Add the rows shown**, the group levels above the lowest one and the Total row add the rows you see. A group that is hidden adds nothing. The lowest group level computes the column from its own rows if one of these is true:

- The column's **Summarize** choice is not **None**, for example **Sum** or **Largest**. A field that you add with **Add as total** also has this choice.
- The column's formula has a total part, such as **Sum of**.
- The formula reads a column that has a **Summarize** choice or a total part.
- The formula reads a column that is set to **Add the rows shown**.

If none is true, the lowest group level adds the values of its rows.

For example, a Debit column shows an account's balance when the balance is above zero. Each account row shows its own balance. The Total adds the account rows.

## Hide groups

To show only some groups, open **Group and sort**, find the level, and add a condition under **Show only groups where…**. A condition can test a total of a field, a column, or a field of the group's record. Turn on **Ask when the report runs** to let the reader set the value from a chip.

## Group by a linked record

To group by a linked record (for example each team of a sale), add the link as a column: in **Add column**, pick **Team (record)**. In **Group and sort**, select **Add group level** and set **Group by** to that column. The group row shows the record's name. To show another field, or a formula, change **Label**. On any level, turn on **Show every Team, even with no rows** to list records that have no rows, and add conditions to choose which. Under another group, a condition can be **Same as the Region above** (the name of the group level above), so each region lists only its own teams.

## Sort

To sort, open **Group and sort**, select **Add sort**, and pick the field. To sort groups by a field of their record, pick it by name (for example **Team › Name**). To sort by anything else, pick **Formula…**. A sort formula that reads a row's fields sorts the detail rows only. Group levels skip it.

## Look up a value

To read one value from another kind of record, add a formula column and pick **Look up**. Choose the **Source**, a condition under **Where**, and the **Value**. The report reads only records you may see. When several records match, **Which match** picks one: **Smallest** or **Largest**.

## Opened rows

When you open a group, its rows are labelled by the first column. To label them another way, turn on **Label opened rows with a formula** in **Columns**. The formula's label takes the first column's place, so opened rows do not show that column's value. A pivot report has no opened rows, so it does not show this choice.

Some columns only name the group, for example an account's number on each line of the account. To show such a column only for the group, set **On opened rows** to **Leave blank** for that column in **Columns**. The value then shows on the group row, or on the group's subtotal row when the group is open and has subtotals. The opened rows show an empty cell, also on a phone. This choice shows only when the report has a group level and is not a pivot report. It does not show for the column that labels the opened rows. It also does not show for the first column of child rows (**Show as child rows**), because that column labels the child rows. Use it for a column that reads a field of the group's record (a **Group field** formula). A plain field that is not the group's field has no value on the group row, so a blank column of that kind shows nothing.

## Filters that list records

A condition on a linked record lets you pick records by name. Turn on **Ask when the report runs**: the reader picks records from a list and can type in **Search** to narrow it. To list the records in groups, choose a field under **Group the list by** (for example a team's region). The reader can then pick a whole group at once with **All** and the group's name. To use other words for that choice, type them in **Whole group text**, with `{group}` where the group's name goes. For example, `All {group} teams` shows "All North teams".

To change the order of the groups, use **Order the groups by**:

- **Name (A–Z)** lists the groups by name. This is the default.
- **The field’s own order** lists the groups in the order that the field gives its choices. For example, account types show as Assets, Liabilities, Equity, Income, Expense. This choice shows only when you group the list by a field with fixed choices.
- **A field** lists the groups by a field that you choose. Each group takes the smallest value of that field among its records, and the smallest comes first. A group with no value comes last. For example, choose a sort number that each record has.

Records with no group always show first. In each group, the records show by name.

## Fold groups

To open a group level when the report opens, turn on **Open when the report opens** for that level. **Collapse all** closes the groups until you open them again.

While a report runs again, "Updating…" shows in the header. On phones and tablets it shows beside **Close** in the builder. The table does not move.

## Formulas

A formula is built from parts you pick from menus: **Number**, **Text**, **Column**, **Field**, **Group field** and **Asked value**; **Add**, **Subtract**, **Multiply**, **Divide** and **Round**; totals such as **Sum of** and running values such as **Running sum**; comparisons, **All of**, **Any of**, **Not** and **Is empty**; **If** and **First match**; text and date parts; and **Look up**.

## Narrow screens

On a phone or tablet, a row too wide for the screen wraps: the label on the first line and the figures below it, each under its heading. To choose where each column goes, open **Arrange**, pick **Phone**, **Tablet** or **Desktop**, and turn on **Arrange columns for this width**. The heading row and the open group rows stay at the top while you scroll.

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
example after a package was removed. Open it to see what is missing. When only a
part of it is gone, such as a field, its owner or an editor can fix it with **Edit
report**. When the kind of report itself is gone, the report says "This kind of
report is not installed" and cannot be edited until that package is installed
again. A person who lost access to a report sees "You no longer have access to
this report".
