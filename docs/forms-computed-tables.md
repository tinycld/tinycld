# Forms: computed fields and tables

Two field kinds extend the forms engine: `computed` (a formula over other fields and table aggregates) and `table` (rows with typed columns, some computed). Hosts may declare `computed` and `table` built-ins.

- Evaluation uses a closed expression tree, not JSON Logic. Numbers coerce as: a number is itself, `true` is 1, anything else is 0. Division by zero is 0. Rounding is half away from zero, at the field's `decimals` (0 to 6).
- The server recomputes on every write and overwrites client-sent values.
- A value that reads a table missing from the write, or a skipped value, is left as it was; the rows' owner computes it.
- Cycles are rejected when a definition is saved.
- One JSON fixture (`core/lib/forms/fixtures/computed-fixture.json`) runs through the TypeScript and Go evaluators.

## Host table columns

A host table built-in's column takes the keys of a custom table column, and two that only a host may set (a custom column an admin edits drops them):

- `component`: the name of a component in the host's UI module `components` map that draws the cell.
- `flex`: the column's width weight on the wide grid, a positive number up to 12 (default 1). The header text and each row's cell slot under it take that weight; a cell drawn in another column's slot (`columnAt`) and an empty slot take the weight of the header column above them. The phone card layout ignores it.

Both are presentation only. The Go server never reads `component`; it decodes `flex` (`Column.Flex`) so the host file round-trips, and never reads it.

## Row kinds

A table built-in may declare `rowKinds`: the kinds of row it holds (an item line, a description line). A row stores its kind in its `kind` cell, and draws and validates only its kind's columns. A table without `rowKinds` has one kind with every column. Each kind has:

- `key`, `label`, and `addLabel` (the label of the action that adds a row of this kind).
- `columns`: the table columns a row of this kind draws, in order.
- `required` (optional): the columns a row of this kind must fill, beside each column's own `required`. Each key must be one of the kind's `columns`, named once.
- `span`: the kind's one cell spans the whole row.
- `columnAt`: a column of this kind → the first kind's column whose header slot it is drawn in on the wide grid.
- `shortcut`: the key combo that adds a row of this kind after the focused row.

A column is required on a row when the column is table-wide `required` (on every kind that draws it) or its key is in the row's kind `required`. A hidden, toggle, or computed column is never required. A text, select, or date cell that is empty or only spaces counts as blank. The missing cell gets `<Label> is required` under the cell, the same text as a table-wide required column.

Core never sees a host table's rows: they live in the host's own collection, written by the package. A package's Go row hooks call `(*forms.Builtin).RowError(row)` on each row (`row` maps column keys and `kind` to cell values). It returns `<Label> is required` for the first required column (in the kind's column order) that is blank, `""` for a complete row, and `<Table label>: unknown row kind "<kind>"` for a row whose kind is missing or not one of the table's kinds.
