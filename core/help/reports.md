---
title: Reports
summary: Make a report from your data, run it, share it, and publish it to everyone
tags: [reports, report builder, share report, publish, filter, group, pivot, formula, totals, groups, lookup, formulas, phone, sort, opening row, all dates]
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

To show something other than the total on group and total rows, open the column in **Columns** and set **On group and total rows show** to **A formula** (the default is **The column's total**). Build the formula from totals, group fields, lookups and asked values. Opened rows still show the column's own value. This choice does not show while **Total** is **Add the rows shown**. A column that a group level groups by cannot have this formula, unless the level groups its dates by a period such as a month. A formula that reads a group field has no value on the Total row, because the Total row is not inside a group.

## Hide groups

To show only some groups, open **Group and sort**, find the level, and add a condition under **Show only groups where…**. A condition can test a total of a field, a column, or a field of the group's record. Turn on **Ask when the report runs** to let the reader set the value from a chip.

## Group by a linked record

To group by a linked record (for example each team of a sale), add the link as a column: in **Add column**, pick **Team (record)**. In **Group and sort**, select **Add group level** and set **Group by** to that column. The group row shows the record's name. To show another field, or a formula, change **Label**. On any level, turn on **Show every Team, even with no rows** to list records that have no rows, and add conditions to choose which. Under another group, a condition can be **Same as the Region above** (the name of the group level above), so each region lists only its own teams.

## Sort

To sort, open **Group and sort**, select **Add sort**, and pick the field. To sort groups by a field of their record, pick it by name (for example **Team › Name**). To sort by anything else, pick **Formula…**. A sort formula that reads a row's fields sorts the detail rows only. Group levels skip it.

## Sort a report

To sort the rows by a column while you read, select the column's heading. The first press sorts from low to high, the next press from high to low, and the third press puts the report's own order back. An arrow beside the heading shows the order. The rows sort inside their groups; the groups keep their order. A running value, such as a running balance, keeps the report's own order, so its figures do not change. A column that shows a running value or a percent has no sort control, and neither does a column of child rows (**Show as child rows**). A pivot report has no sort control. **Copy link** keeps the sort. If a link sorts by a column that the report no longer has, the report shows why and a **Clear sort** button; select it to show the report in its own order.

## Look up a value

To read a value from another kind of record, add a formula column and pick **Look up**. Choose the **Source**, a condition under **Where**, and the **Value**. The report reads only records you may see. Under **Which match**, choose what the lookup gives:

- **Smallest** or **Largest**: one value of the records that match.
- **Add up**: the sum of the values of all records that match. With no match, the cell is empty. If the records that match have amounts in two currencies, the cell is empty and the report shows a note.
- **Count**: the number of records that match and have a value. With no match, it is 0.

Inside **Where** or **Value**, pick **This row's field** to compare with a field of the report's own row. For example, "Day = This row's Day" finds the records of the same day as each row. On group and total rows, **This row's field** has no value.

## Opened rows

When you open a group, its rows are labelled by the first column. To label them another way, turn on **Label opened rows with a formula** in **Columns**. The formula's label takes the first column's place, so opened rows do not show that column's value. A pivot report has no opened rows, so it does not show this choice.

Some columns only name the group, for example an account's number on each line of the account. To show such a column only for the group, set **On opened rows** to **Leave blank** for that column in **Columns**. The value then shows on the group row, or on the group's subtotal row when the group is open and has subtotals. The opened rows show an empty cell, also on a phone. This choice shows only when the report has a group level and is not a pivot report. It does not show for the column that labels the opened rows. It also does not show for the first column of child rows (**Show as child rows**), because that column labels the child rows. Use it for a column that reads a field of the group's record (a **Group field** formula). A plain field that is not the group's field has no value on the group row, so a blank column of that kind shows nothing.

## Opening rows

To start each group with a row of its own, open **Group and sort**, find the level, and turn on **Start each group with a row**. Enter a **Row label**, for example "Beginning balance". For each column after the first, select **Add a value** and build a formula from totals, group fields, lookups and asked values, or leave it with no value. Child row columns have no value on this row. The row shows first under each group that you open. It also shows under a group that has no rows of its own, if the level shows every record with **Show every …, even with no rows**. A closed group and a group that **Show only groups where…** hides have no opening row. The last group level of a pivot report cannot have an opening row.

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

A formula is built from parts you pick from menus: **Number**, **Text**, **Column**, **Field**, **This row's field** (inside a lookup), **Group field** and **Asked value**; **Range start or end** (the first or last day of a period you ask for, such as "Period start"); **Add**, **Subtract**, **Multiply**, **Divide** and **Round**; totals such as **Sum of** and running values such as **Running sum**; comparisons, **All of**, **Any of**, **Not** and **Is empty**; **If** and **First match**; text and date parts; and **Look up**. When the period is **All dates**, "Period start" and "Period end" have no value, so a comparison with them matches no record.

## When columns do not fit

A report never scrolls sideways. Each column gets the room its heading and its values need. Numbers, amounts, percents, dates and yes/no values are never cut short, and short text such as an entry number is not either. Long text gets narrower first and ends in "…". Spare room goes to the columns whose text is cut, then to the text columns. A column with no value is as wide as its heading.

Group, opening, subtotal and total rows show their whole label: it runs across the empty cells after it. When the rows wrap, a total's label moves onto the line of its figures if there is room.

When the columns do not fit on one line, the report uses the first arrangement that fits the screen, widest first: **Desktop**, then **Tablet**, then **Phone**. With no arrangement that fits, each row wraps by itself: as many columns as fit go on each line, the label first, and the next lines start under the row's label.

To choose where each column goes when one line does not fit, open **Arrange**, pick **Phone**, **Tablet** or **Desktop**, and turn on **Arrange columns for this width**. Inside a line, each column gets the room its content needs; a column that spans more tracks gets more of the spare room. The heading row and the open group rows stay at the top while you scroll. A long report loads more rows as you scroll, also when it has groups.

## Run a report

- The chips above the report set its period and filters; **Add filter** adds a chip
  for a filter you can set while you read. In the period chip, **All dates** shows
  every row, also a row with no date. A link copied with **Copy link** opens the
  report with the same choices.
- To sort, press a column heading: each press sorts up, then down, then back to the
  report's order. When the rows wrap (on a phone), use the **Sort** chip above the
  report instead.
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
