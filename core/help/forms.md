---
title: Forms and record types
summary: How a record's fields, pages and rules are defined and changed
tags: [forms, fields, rules, types]
order: 45
---

Some records, such as a task or a ticket, carry a **form**. The form decides
which fields the record shows, in what order, on which page, and which rules
apply. A record type is a form: a project can have a Task form and a Bug
form, and each record uses one of them. Forms belong to the organization, so
every member sees the same record types.

## Fields

A form has the record's **built-in fields** and any **custom fields** an
admin adds. A built-in field can be renamed, moved, hidden or given a
default, but not deleted. A built-in field that the record requires, or that
the record locks, cannot be hidden: its **Hidden** switch cannot be changed,
and shows "Required by the record" or "Locked by the record". A rule cannot
hide it either. A custom field can be text, long text, a number, a
date, a toggle, a choice list, a person, a checklist, rich text, markdown, a
file, a **table** or a **calculated value**. The feature that owns a record
can also give it its own table or calculated value; you can move and rename
those, but not change their columns or formula.

Each field has a width: full, half, third or quarter of the form, or fit,
the width of the field's content (never wider than the form). On a narrow
screen a quarter becomes a half, halves and thirds become full width, and a
fit field keeps its content's width. A field can also have its own width for
narrow screens, which it then uses there instead.

## Tables and calculated values

Some records have a **table**, such as the lines of an order. Each row of a
table has the same columns.

- To add a row, select **Add row** below the table. Some tables have more
  than one kind of row, such as an item line and a note line. Their first
  kind has its own add button below the table, and **Add…** next to it
  lists the other kinds.
- In some tables, press Enter in the last row's cell of a column such as
  Qty to add a row.
- Each row has a menu button (⋮) at its end. Its label is "Line N actions",
  where N is the number of the row. Select it to open the row's menu:
  **Insert line above**, **Insert line below**, **Move up**, **Move down**,
  **Duplicate** and **Remove**. The first row cannot move up, and the last
  row cannot move down. With a keyboard, press Tab to go to the menu button,
  then press Enter or Space.

After a row action, the focus goes where you can continue:

- After **Move up** or **Move down**, the focus stays on the moved row's
  menu button, so you can open the menu again to move the row more.
- After **Remove**, the focus goes to the next row, else the row above,
  else the add button.
- After **Duplicate** or an insert, the focus goes to the new row.

Some tables also have row keys. They work while the focus is in a cell or on
the menu button of a row, and act on that row:

| Keys | Action |
|---|---|
| ⌘⌥↑ | Move the row up |
| ⌘⌥↓ | Move the row down |
| ⌘⌥D | Duplicate the row |
| ⌘⌥⌫ | Remove the row |
| ⌥M | Open the row's menu |

A key moves or removes the row and keeps the focus in the same column. When
a table has its own key for a kind of row, such as ⌥T for a note line, the
**Add…** menu shows the key, and the key adds that kind of row below the row
that has the focus, or at the end when the focus is not in the table. Press ⇧? to see the keys of the screen you are on.

A star after a column name shows a required column. If a required cell is
empty when you save, the form shows an error below that cell. On a phone,
each row shows as a card, with the column names above the values.

A **calculated value**, such as a line amount or a subtotal, changes while
you type. You cannot edit it. While the rows of a record load, the table
shows "Loading rows…" and the totals show their saved values.

## Pages

Fields are grouped into pages. When a record is created, the pages show one
at a time, with Next and Back. In a single-line field, press Enter to go to
the next page. In the record's detail view every page shows as a section.

## Rules

A rule reads one or more fields and changes the form: it can show or hide a
field or a page, make a field required, change the choices of a list, or set
a value. Rules run in order, once, on the current values. A rule cannot
trigger another rule, so rules never loop. A hidden field is never required.

## Defaults

A field can start with a fixed value, today's date, a date a number of days
from today, or the current user.

## Changing a record's type

Where the feature that owns the record offers it, open the record's actions menu and choose **Change type**.
Values for fields the new type does not have are kept but hidden; they show
again if the record returns to the old type or the new type gains that field.

Admins manage forms under Settings. See [Automation rules](help://core:rules)
for rules that run when records change, which are a different thing.

## Managing forms

An owner or admin opens **Settings → Forms**. With one record type the list opens directly; otherwise pick the record type first. The list shows every form of that type. Drag the handle to reorder the type selector, and use the menu on a row to rename, duplicate, make default, archive or delete a form. The default form cannot be deleted or archived; a form that records still use cannot be deleted, archive it instead.

To collect forms from a website, see [Web forms](help://core:web-forms).

## Editing a form

The editor shows the form as users see it. Select a field to edit its label, width (Full, Half, Third, Quarter or Fit), its width **On narrow screens** (**Default** keeps the rule above), requirement, default and kind-specific settings in the panel. Drag a field by its handle to move it, also onto another page: a chip with its label follows the pointer and a bar shows where it lands. On a phone or a tablet, hold the handle for a moment before you drag. To move a field without dragging, use **Page**, **Move up** and **Move down** in its panel. **Add field** at the end of a page asks for the field's label, then adds a field of the chosen kind and opens its panel. **Add page** appends a page; a page can be deleted only when it is empty.

To add a table or a calculated value, select **Add field**, then **Table** or **Calculated value** under **Tables and totals**. A new table has one text column, Item. A new calculated value starts at 0.

To change the columns of a table, select the table. Its panel shows **Columns**:

- To add a column, select **Add column**, select its kind (Text, Number, Toggle, Date, Choice list or Calculated), then type its name in the box that opens. Formulas refer to the column by this name, and its kind cannot be changed later.
- To edit a column, select its name ("Edit column Qty"). Its settings open inside its own card, right under its name: **Label**, **Required**, **Hidden**, and the settings of its kind, such as **Minimum** and **Maximum** for a number, **Maximum length** for text, or **Options** for a choice list. The key and the kind are set when you add the column, and do not change.
- To move a column, select its up or down arrow button ("Move column Qty up"). To remove a column, select its remove button ("Remove column Qty"), then confirm. A formula that uses the removed column shows it as missing. A table always keeps one column. When two columns have the same label, these buttons also give the column's place ("Remove column 3 (Price)").

With a keyboard, press Tab to go to a button or a menu, then press Enter or Space.

A calculated value and a Calculated column have a **Formula**. You do not type a formula. You build it from parts:

1. Select the part menu, then select the kind of part: **Number**, **Field** (**Column** in the formula of a column), **Add**, **Subtract**, **Multiply**, **Divide**, **Round**, **Smallest of**, **Largest of**, **Sum of a column** or **Number of rows**.
2. For a Number, type the number. For a Field or a Column, select which one. For Sum of a column, select the table and its column. For Number of rows, select the table.
3. Add, Multiply, Smallest of and Largest of hold two or more parts, shown below them. Select **Add part** to add a part, and its remove button ("Remove part 2 of Add") to remove one. A screen reader reads the place of each part, for example "Number, part 2". Subtract and Divide hold two parts. Round holds one part; select its number of places.

The line below **Formula** shows the formula as text, for example "= Qty × Price". **Decimal places** sets how the result is rounded. A formula can read numbers, toggles (on is 1) and other calculated values. The formula of a column can read only the other columns of the same row. Only a calculated value can add up a column of a table or count its rows.

> **Note:** If two formulas read each other, the editor shows the problem below the formula. If you delete a field or a column that a formula reads, the formula shows it as "(missing)", and you cannot save the form until you change the formula.

A field's key is set when it is created and never changes, so renaming a field keeps its stored values. Deleting a custom field keeps the stored values hidden and removes any rule, default or requirement that named it; the editor says what it removed.

The **Rules** tab lists the form's rules. Each rule has conditions ("when all of" or "when any of") and actions. Pick a field, an operator and a value; then pick an action and its target. **Set options** is offered only for choice lists. **Hide** is not offered for a field the record requires or locks, nor for the page that holds one. Write a date as YYYY-MM-DD. A problem with a rule shows in that rule's card.

**Preview** opens the draft as a desktop sees it, with rules live; switch to Phone to see how its columns stack on a small screen. **Save** writes the form; the server checks it again and points at any field it refuses. The list above the form names each problem by its field or rule.

If you leave the editor with unsaved changes, it asks before it discards them: on the Back arrow, the back gesture or button, the browser's Back and Forward buttons on the web, and when you reload or close the browser tab. The browser's buttons ask only while the editor is on screen: if you go to another screen or another TinyCld app first, a browser Back or Forward from there does not ask, so save before you leave the editor. If someone else saves the form while you edit it, the editor keeps your changes and says so; **Reload** drops your changes and shows their version.
