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
     order of the rows. A level over a date can group by day, month, quarter, year
     or fiscal year; its group rows are named in words, such as "Jan 2026",
     "Q1 2026", "2026" or "FY 2026".
   - **Pivot**: spread a column with a total across one column per value, such as
     one column per month, or one column per linked record, such as one column per
     team. Under **Show as**, choose the record's field that names its columns. A
     column of a month is headed in words, such as "Jan 2026". Under **Columns**,
     choose **Show every month of …** to show every month of a period, and turn on
     **Total after the pivot columns** to show the total after them.
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

## Group by levels some rows do not have

To show rows that have no value at a level directly under the group above, open the level in **Group and sort** and set **Rows with no <column>** to **Place them directly under the group above**. Those rows then have no group or total of their own at that level, and they count in the total of the group above. The first and the last level cannot do this. A level grouped by a date bucket cannot either.

## Column headings that name a day

To make a column's heading show a value from the report's parameters (for example the **As of** day), turn on **Heading: from a parameter** in **Columns** and choose the parameter. The heading can be text, a day, a date and time, or a number. A day shows in short form ("Nov 8, 2026"). Money, yes/no, a date range and a list cannot be a heading. **Customize** and the builder still list the column by its name. If the heading has no value, the heading shows the column's name. This is the case when the parameter is not set, is empty, or is text with only spaces.

## Statements: sections and computed rows

A statement shows fixed sections and rows computed from them. A computed row can read the report's parameters. To put a computed row inside a section, choose the section in **Add a row to this section**. The row shows after the section's rows and before its total, while the section is open. The section's total adds the row, and a row later in the layout that reads the section reads that total. A row inside a section cannot read its own section. A section with a row inside it can always open, also when every one of its own rows is hidden, so that its total is never more than the rows you can see.

A computed row can have its own formula in a column: add it under **This column: …**. A number column the row names no formula for uses the row's formula. To use only the column formulas, press **Remove** beside the row's formula; those other columns are then blank. A column whose formula reads only other columns of the same row (a Change of "Current − Prior year", a percent change) is worked out from that row's own figures, on computed rows and on section totals too.

## Show a row as a percent of a section

In a statement, a column can show each row as a percent of one section's total, such as each expense as a percent of income. To add one, select **Add formula**, choose **Percent of a section's total**, choose the column to divide, then open the menu that follows it, which shows a section's name, and choose the section to divide by. Each row shows its own figure divided by that section's total, times 100: the rows of each section, the group and section totals, and the computed rows. The section's own total shows 100%. When the section's total is 0 or empty, the column is empty. When the report has more than one currency, each currency divides by its own section's total. On a pivot report the percent divides the row's total across the pivot columns. This part is offered only as a column's whole formula in a report with sections, and no other formula can read that column.

## Pivot by month across a whole period

A pivot over a date shows one column for each day, month, quarter, year or fiscal year that has rows. To show every month of a period, also a month with no rows, open **Pivot**, choose the date field and **By** **Month**, and under **Columns** choose **Show every month of …** with the date range the report asks for. A month with no rows then shows 0 in a sum, and 0.00 when the sum is an amount. When the period has more months than **At most this many columns**, the report shows the first ones and tells you how many it left out. When the period is **All dates**, the report shows only the months that have rows.

Column headings name each bucket in words: a month as "Jan 2026", a quarter as "Q1 2026", a year as "2026", a fiscal year as "FY 2026" (the year in which it ends), and a day in the short day form, such as "Jan 5, 2026".

The pivot columns usually come after all the other columns. Turn on **Total after the pivot columns** to show them just before the column with the total, so that each row's total follows its months.

## Hide groups with nothing to show

To hide a group when every row in it is hidden (for example by **Show only groups where…**), turn on **Hide a group when every row in it is hidden** for its level. The group and its total do not show. A group that has a row placed directly under it from a level below stays. This chains upwards: a group whose child groups are all hidden is hidden too. This choice shows for a level grouped by a date bucket too. The last level cannot do this, and neither can the first level of a statement, because its sections always show. A total set to **Recompute from all rows** still counts the rows of the hidden groups, as it does for **Show only groups where…**. A total set to **Add the rows shown** does not.

## Group by a formula

To group or sort by a value you do not want to show, add the formula as a column and turn on **Use only to group or sort (do not show)**. The report still groups and sorts by it, but the column has no heading or cells, is not listed in **Customize**, and is not in an export. The first column cannot be hidden, and neither can the first column of child rows, because it names them. Neither can a column that a width layout places, a column that the opening row of a group level has a value for, or the cell column of a pivot.

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

To open a group level when the report opens, turn on **Open when the report opens** for that level. To close or open one group, select its row. The other groups stay as they are, also the groups that load when you scroll further. **Collapse all** closes the groups until you open them again.

While a report runs again, "Updating…" shows in the header. On phones and tablets it shows beside **Close** in the builder. The table does not move.

## Formulas

A formula is built from parts you pick from menus: **Number**, **Text**, **Column**, **Field**, **This row's field** (inside a lookup), **Group field** and **Asked value**; **Range start or end** (the first or last day of a period you ask for, such as "Period start"); **Add**, **Subtract**, **Multiply**, **Divide** and **Round**; totals such as **Sum of** and running values such as **Running sum** and, in a statement, **Percent of a section's total**; comparisons, **All of**, **Any of**, **Not** and **Is empty**; **If** and **First match**; text and date parts; and **Look up**. When the period is **All dates**, "Period start" and "Period end" have no value, so a comparison with them matches no record.

## When columns do not fit

A report never scrolls sideways. Each column gets at least its least width. Numbers, amounts, percents, dates and yes/no values are never cut short, and short text such as an entry number is not either. Long text gets narrower first and ends in "…". On one line, the rest of the room goes to the columns by their **Extra room**: while two columns both need more room for their text, a column with **Double** gets twice the room of a column with **Normal**, also when its text is shorter. A column stops at the width its values need while another column's text is still cut, and its share goes to the others; when every column shows its whole text, the room left goes by **Extra room** again. A column with **None** gets the room that its values need, and no more. A column with no value is as wide as its heading.

Group, opening, subtotal and total rows show their whole label: it runs across the empty cells after it. When the rows wrap, a total's label moves onto the line of its figures if there is room.

When the columns do not fit on one line (or the rows are narrower than **One line from**), the report uses the first arrangement that fits the screen and that the rows reach (its **Use from**), widest first: **Desktop**, then **Tablet**, then **Phone**. With no arrangement that fits, each row wraps by itself: as many columns as fit go on each line, the label first, and the next lines start under the row's label. A column heading that is too long for its column uses two lines. When the rows wrap, the lines after a row's first line are shorter, so more rows fit on the screen. Notes about the report, such as a pivot that cannot follow an arrangement, show in one band above the report. On a phone the band shows two lines; select **Show all** to read every note.

To change how a column fits, open the column in **Columns**:

- **Narrowest** is the least width a text column gets before the report wraps its rows: **Automatic** (about eight letters), **Whole value** (never cut), or a number of letters.
- **Long text** keeps the text on **One line, ends in …**, or wraps it to 2 or 3 lines. A column that wraps is never narrower than its longest word, whatever its **Narrowest** setting, so its words stay whole.
- **Extra room** is the column's share of the spare room on one line: **None**, **Normal**, **Double** or **Triple**. A column with **Double** gets twice the spare room of a column with **Normal**, whichever column has the longer text, but no column takes room past its values' width while another column's text is still cut. Text and the first column start at **Normal**; other numbers, amounts, percents, dates and yes/no values start at **None**. In a pivot, the setting applies to every column the pivot makes from that column.

Numbers, amounts, percents, dates and yes/no values show **Never cut**: you can change only their extra room.

In **Arrange**, **One line from** is the least row width at which the report uses one line; below it, the report uses an arrangement or wraps its rows. **Use from** on an arrangement is the least row width at which it is used. On a computer the rows are the window's width less the side rail.

To choose where each column goes when one line does not fit, open **Arrange**, pick **Phone**, **Tablet** or **Desktop**, and turn on **Arrange columns for this width**. Inside a line, each column gets the room its content needs; a column that spans more tracks gets more of the spare room. The heading row and the open group rows stay at the top while you scroll. On a phone, the title, the chips and the notes scroll away with the rows, so the rows get the screen; scroll back to the top to change a chip. A long report loads more rows as you scroll, however many groups it has. Until its last rows have loaded, the list ends with **Loading more rows…**; the Total shows with the last rows, and **End of report** marks the end. If more rows cannot load, the list ends with **Could not load more rows**. Select **Retry** to try again. The rows that show stay.

## Run a report

- The chips above the report set its period and filters; the period chip names the
  period, such as **This month**, and the dates show under the chips. **Add filter**
  adds a chip for a filter you can set while you read. On a narrow screen, swipe the
  chips sideways to see the rest; the right edge fades while there are more. In the period chip, **All dates** shows
  every row, also a row with no date. A link copied with **Copy link** opens the
  report with the same choices.
- To sort, press a column heading: each press sorts up, then down, then back to the
  report's order. When the rows wrap (on a phone), use the **Sort** chip above the
  report instead (on a phone, scroll back to the top to reach it).
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
