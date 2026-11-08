# Report Column Fit, Spec v4 — Part 2 (Tasks 8–13) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The client reads the column fit that spec version 4 holds: the TS spec and the builder draft keep `display.oneLine`, `display.columns` and a layout `minWidth`; the measure and the fit apply them (min, max, fixed and % widths, wrap to N lines, flexGrow, the one-line and per-layout width gates); the table says which rule laid it out (an e2e marker); text cells wrap; the builder sets the fit; help and docs say how.

**Architecture:** Placement and fit stay in TS (`core/reports/lib`). `column-measure.ts` turns each `ColumnFit` into a `ColumnMeasure` (with `max`, `lines`, `pct`); `table-lines.ts` resolves `%` per line width (`atWidth`), caps growth at `max`, gates one line and each arrangement by width (`fitTable`), and puts `lines` on the `LineLayout`. `useReportView` hands the spec's fit to `useTableLayout`; `ReportTable` writes `data-fit-width` and `data-layout` once measured; `ReportRow` draws `numberOfLines` from `layout.lines`.

**Tech Stack:** TypeScript (zod v4, vitest), React Native + react-native-web 0.21, Playwright.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/column-fit-design.md` (§3.1–3.2 fields and CSS meaning, §3.3 figures never cut, §3.5 marker, §3.6 builder, §3.7 Books settings) with the binding decisions `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/column-fit-decisions.md` (option B; no TanStack; marker; answers 1–6; **no backward compatibility**). Outline: `docs/plans/2026-11-08-report-column-fit-v4-outline.md`. Part 1 (`docs/plans/2026-11-08-report-column-fit-v4-part-1.md`, Tasks 1–7) is done first.

All paths are relative to the core repo `/Users/nas/tmp/tinycld/books-purchases/tinycld` unless absolute. Line numbers are at main `f018575f`; Part 1 moves some (named by symbol too).

**Part 1 end state this part builds on:** Go accepts and stores `display.oneLine`, `display.columns`, `display.layouts[].minWidth` and refuses any fit that could cut a figure (`neverCutMessage`); Go no longer places cells; `CURRENT_SPEC_VERSION = 4` and TS `reportSpecSchema.version` is `z.literal(CURRENT_SPEC_VERSION)`; every fixture says version 4; `ARRANGEMENT_NOTICE` exists in `table-lines.ts` and `authoredLines` returns `{ kind: 'refused', notice: ARRANGEMENT_NOTICE }` for an arrangement it cannot place; `fromVersion` is gone. The TS `display` schema still has `layouts` only (Task 8 adds the rest).

---

## For review

### Changes from the outline

1. **Six tasks, not seven.** Outline Task 14 is folded: the `docs/packages.md` rows go with the schema (Task 8, where the fields appear), the help with the last user-facing task (Task 13), and the column-fit e2e with the builder (Task 12) and wrap (Task 13) tasks that make it reachable. The outline's ArrangeSection fix (keep `oneLine` when an arrangement changes) moves to Task 8, where `oneLine` enters the draft, so no task leaves the builder dropping it.
2. **Builder before wrap (12 ↔ 13 swapped).** The wrap e2e sets "Wrap to 2 lines" in the builder, so the builder comes first.
3. **Builder pickers follow design §3.6, not the outline's list.** A text column gets **Narrowest** (`minWidth`), **Long text** (`whiteSpace` + `lineClamp`) and **Extra room** (`flexGrow`); a figure shows **Never cut** and only **Extra room**. `width`, `maxWidth` and `%` lengths stay JSON/CLI-only (the measure and fit read them; Task 9/10 tests cover them).
4. **Headings do not change.** `HeadingText` already wraps to 2 lines (`numberOfLines={2}`); the outline's "one-line heading reads `layout.lines`" is dropped.

### Choices this part makes (not in the decisions)

- **Units.** A `ch` or `px` length is the text's width; the cell's padding is added (a label's chrome: indent, deepest start, right padding). A `%` length is the cell's whole width, as a part of the line it is on (the row width on line 1, the row width less the continuation indent on later lines).
- **CSS order.** `width` sets natural = min = max; `minWidth` then raises min (never under the heading's longest word); `maxWidth` caps natural and max but never under min. A `%` width never goes under the column's automatic min.
- **Wrap.** `whiteSpace: normal` without `lineClamp` wraps to 2 lines (`DEFAULT_LINE_CLAMP`). A wrapping column's automatic min is the larger of its longest word and eight letters (capped at its natural width), so a wrap never cuts a word.
- **flexGrow — deviation from design §3.2, needs the human's sign-off.** The design says `flexGrow` applies "on one line and on automatic lines". This plan applies it on one line only. Inside an arrangement or automatic lines a side part's share stays its `span` (tracks), as today; the `max` cap applies everywhere. Why: on an automatic line of figures only (a phone's "debit · credit" line) every grow is 0, so `shareWidth` would give all spare room to the first part (the remainder rule), which moves the figures off their headings; today tracks spread it evenly. If the human wants the design's rule, the follow-up is: in `autoLines`, a side part's grow is its cell's measure grow when some part on that line has grow > 0, else its tracks (one change in `fitBlock`'s `shares`), with the table-lines auto-line width cases checked again.
- **Gates.** `display.oneLine.minWidth` and `layouts[].minWidth` compare with the measured row width (window less the rail on `/a/reports`). A layout skipped by its gate gives no notice (it is not refused).
- **One default changes (outline):** a label column whose own type is a figure (a money, number, percent, date, datetime or bool first column) is never cut, as a typed label already is.
- **An empty fit is dropped.** A builder save writes no `{}` for a column whose fit is empty, and writes the fit's keys in Go's order (no backward compatibility, so no old byte layout to keep).
- **The builder's figure check** (`fitIssues`) uses `columnTypes` (a column's detail type after its aggregate). Go's wider rule (a typed record label, a typed `detail.label` formula) still refuses at save; that problem comes back keyed `display.columns.<id>.<setting>` and shows under **Columns**.

### Tests this part breaks

Grep basis: `tests/e2e/reports-*.spec.ts`, `core/reports/**/__tests__/*.test.ts`, `books/tests/e2e/general-ledger.spec.ts`, `books/tests/e2e/trial-balance.spec.ts`.

| Assertion | Broken by | Fixed in |
|---|---|---|
| `core/reports/lib/builder/__tests__/errors.test.ts:15` still holds (`display.layouts…` → `arrange`); no break | — | — |
| `core/reports/lib/__tests__/column-measure.test.ts` (all `measureColumns` cases): no break expected — every label column there is text, and `fitted` returns exactly `{ natural, min, grow }` with no fit | T9 | — |
| `core/reports/lib/__tests__/table-lines.test.ts` `tableLines(...)` `toEqual` cases (`:231-330`): no break — `lines` is added only when a measure has `lines > 1`, and `shareWidth` gives the same widths when no part has `max` | T10 | — |
| `tests/e2e/reports-fit.spec.ts:127` (`toHaveCSS('opacity', '1')`): kept; Task 11 adds `waitForTableFit` after it | — | T11 |

No existing core e2e asserts arrange hint text or the "When columns do not fit" help text (grep: only `reports-wrap.spec.ts:968` matches "columns do not fit", and it reads `PIVOT_NOTICE`, unchanged). Books e2e (`general-ledger.spec.ts`, `trial-balance.spec.ts`) read no `data-fit-width`/`data-layout` and set no fit; the first column of GL and TB is `account` (text), so the label default change does not reach them. Books' hand-built `ReportViewModel` (`books/tinycld/books/screens/helix-mockup.tsx`) has no `fit`; Task 11 makes `fit` optional so it still compiles. Part 2 breaks no Books test.

---

## Global Constraints

- Spec v4 only (Part 1). TS field names and value sets as Go (`core/server/reports/fit.go`): length `^(\d{1,4}(\.\d+)?)(ch|px|%)$`; `width`: length; `minWidth`: `auto` | `min-content` | `max-content` | length; `maxWidth`: `none` | length; `whiteSpace`: `nowrap` | `normal`; `lineClamp`: 1–4; `textOverflow`: `ellipsis`; `flexGrow`: integer 0–10; `display.oneLine.minWidth` and `layouts[].minWidth`: px only. Go checks ranges; TS reads what Go stored.
- Go's JSON order: `Display{oneLine, columns, layouts}`, `DisplayLayout{width, minWidth, tracks, cells}`, `ColumnFit{width, minWidth, maxWidth, whiteSpace, lineClamp, textOverflow, flexGrow}`; a map's keys in byte order. A builder save that changes nothing writes the same bytes.
- A figure is never cut: the measure ignores every setting but `flexGrow` on a figure column (and on a typed or figure label column).
- No `any`, no `biome-ignore`, no `console.*`, no raw hex; JSX minimal (lists built above the return); forms validated by the draft's zod schema (`builderSchema` → `draftIssues`), errors shown with `FieldError` from `getFieldState`.
- E2E: no `page.goto` inside the app; drive the UI; resize with `setViewportSize`; phone run at 390 where the phone path differs (automatic lines; `MobileLayout`).
- Read files with Read; edit with Edit (hooks block `sed -i` and scripted replaces).
- Commands, from `/Users/nas/tmp/tinycld/books-purchases/tinycld`:
  - unit: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run <file>`
  - types: `pnpm exec tinycld-pkg typecheck`
  - lint: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check <files>`
  - e2e (one file): check ports first (`lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7210|7211)'`), then `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- <name>.spec.ts`
- The driver commits; each task's last step lists the files to stage.

## Review Focus

1. **`%` on one line and in authored lines.** Expect: a `%` resolves against the line it is on (row width on one line and line 1; row width − indent on later lines); a `%` width is fixed (min = natural = max). Test: Task 10 "a % width is a part of the line".
2. **Capped growers.** Two or three growers, one with `max`. Expect: the capped one stops at `max`, the others share the rest; with no `max` the widths are exactly today's. Test: Task 10 "stops a grower at its max".
3. **A minWidth wider than the content.** Expect: kept (natural rises to it), and never under the heading's longest word. Test: Task 9 "raises a text column to its minWidth".
4. **No stale marker after a resize.** Expect: `waitForTableFit` returns only when the probe's width equals `data-fit-width`, so a layout of the old width is never read. Test: Task 11 `reports-fit.spec.ts` (4 widths); Task 12 spec (3 widths).
5. **A oneLine gate wider than any screen** (`4000px`). Expect: never one line, an arrangement or automatic lines instead, no crash. Test: Task 10 "never one line under the gate".

---

### Task 8: TS spec v4 schema, builder round trip, format docs

**Files:**
- Modify: `core/reports/lib/spec.ts:187-193` (`displayLayoutSchema`), `:226` (`display`); add the fit schemas after `displayLayoutSchema`
- Modify: `core/reports/lib/builder/draft.ts:138-156` (`columnFields`), `:240` (`draftShape.display`), `:607` (`emptyDraft`), `:622-654` (`columnDraft`), `:693` (call), `:716` (`draftFromSpec` display), `:1026-1027` (`specFromDraft` display)
- Modify: `core/reports/components/builder/ArrangeSection.tsx:69-75`, `:93` (keep `oneLine`)
- Test: `core/reports/lib/__tests__/spec.test.ts`, `core/reports/lib/builder/__tests__/draft.test.ts`
- Modify: `docs/packages.md:1321-1344`, `:1389`

**Interfaces:**
- Consumes: Part 1 `CURRENT_SPEC_VERSION = 4`; `FEEDINGS` (`spec.test.ts:10`, version 4 after Part 1); `CATALOG`, `SALES_KEY` (`catalog-fixture.ts`; fields `customer`, `amount`), `META` (`draft.test.ts:586`).
- Produces (spec.ts): `FIT_LENGTH: RegExp`; `type FitLength = { value: number; unit: 'ch' | 'px' | '%' }`; `parseFitLength(s: string): FitLength | null`; `columnFitSchema`; `type ColumnFit`; `displayOneLineSchema`; `type DisplayOneLine = { minWidth: string }`; `displayLayoutSchema` with `minWidth?: string`; `reportSpecSchema.display = { oneLine?, columns?: Record<string, ColumnFit>, layouts? }`; `type DisplayFit = { oneLine?: DisplayOneLine; columns?: Record<string, ColumnFit> }`.
- Produces (draft.ts): `ColumnDraft.fit?: ColumnFit`; `BuilderDraft.display = { oneLine?: DisplayOneLine; layouts: DisplayLayout[] }`; `tidyFit(fit: ColumnFit | undefined): ColumnFit | undefined` (exported).

- [ ] **Step 1: Write the failing tests**

Append to `core/reports/lib/__tests__/spec.test.ts` (add `parseFitLength` to the import from `'../spec'`):

```ts
describe('column fit (spec version 4)', () => {
    const FIT = {
        ...FEEDINGS,
        display: {
            oneLine: { minWidth: '1040px' },
            columns: {
                cost: { flexGrow: 0 },
                note: {
                    minWidth: '7.5ch',
                    maxWidth: '40%',
                    whiteSpace: 'normal',
                    lineClamp: 2,
                    textOverflow: 'ellipsis',
                },
            },
            layouts: [
                { width: 'tablet', minWidth: '600px', tracks: 2, cells: [{ col: 'note', line: 1 }] },
            ],
        },
    }

    it('reads every fit setting as it is', () => {
        expect(parseSpec(FIT)).toEqual(FIT)
    })

    it('refuses a length with no unit, a width gate not in px, and a value Go never stores', () => {
        const withDisplay = (display: unknown) => parseSpec({ ...FEEDINGS, display })
        expect(withDisplay({ columns: { note: { minWidth: '12' } } })).toBeNull()
        expect(withDisplay({ oneLine: { minWidth: '60ch' } })).toBeNull()
        expect(withDisplay({ oneLine: {} })).toBeNull()
        expect(
            withDisplay({ layouts: [{ width: 'tablet', minWidth: '50%', tracks: 1, cells: [] }] })
        ).toBeNull()
        expect(withDisplay({ columns: { note: { textOverflow: 'clip' } } })).toBeNull()
        expect(withDisplay({ columns: { note: { lineClamp: 5 } } })).toBeNull()
        expect(withDisplay({ columns: { note: { colour: 'red' } } })).toBeNull()
    })

    it('parses a length into its number and unit', () => {
        expect(parseFitLength('7.5ch')).toEqual({ value: 7.5, unit: 'ch' })
        expect(parseFitLength('600px')).toEqual({ value: 600, unit: 'px' })
        expect(parseFitLength('40%')).toEqual({ value: 40, unit: '%' })
        expect(parseFitLength('max-content')).toBeNull()
        expect(parseFitLength('12')).toBeNull()
    })
})
```

Append to `core/reports/lib/builder/__tests__/draft.test.ts` (add `tidyFit` to the import from `'../draft'`, and `type ColumnFit` to the import from `'../../spec'`):

```ts
describe('column fit in the builder', () => {
    const FIT_SPEC: ReportSpec = {
        version: CURRENT_SPEC_VERSION,
        source: SALES_KEY,
        columns: [
            { id: 'customer', label: 'Customer', path: [{ field: 'customer' }] },
            { id: 'amount', label: 'Amount', path: [{ field: 'amount' }] },
        ],
        display: {
            oneLine: { minWidth: '1040px' },
            columns: {
                amount: { flexGrow: 0 },
                customer: { minWidth: '7.5ch', whiteSpace: 'normal', lineClamp: 2 },
            },
            layouts: [
                {
                    width: 'tablet',
                    minWidth: '600px',
                    tracks: 2,
                    cells: [
                        { col: 'customer', line: 1, span: 2 },
                        { col: 'amount', line: 2 },
                    ],
                },
            ],
        },
    }

    it('keeps the one-line width, every column fit and a layout gate through the draft', () => {
        const draft = draftFromSpec(FIT_SPEC, META, CATALOG)
        expect(draft.columns[0]?.fit).toEqual({
            minWidth: '7.5ch',
            whiteSpace: 'normal',
            lineClamp: 2,
        })
        expect(draft.display.oneLine).toEqual({ minWidth: '1040px' })
        expect(builderSchema(CATALOG).safeParse(draft).success).toBe(true)
        // A save that changes nothing writes the same bytes.
        expect(JSON.stringify(specFromDraft(draft, CATALOG))).toBe(JSON.stringify(FIT_SPEC))
    })

    it('writes each fit in the engine order, the columns in byte order, and no empty fit', () => {
        const draft = draftFromSpec(FIT_SPEC, META, CATALOG)
        const shuffled: ColumnFit = { lineClamp: 2, whiteSpace: 'normal', minWidth: '7.5ch' }
        const columns = draft.columns.map(column =>
            column.id === 'customer' ? { ...column, fit: shuffled } : { ...column, fit: {} }
        )
        const saved = specFromDraft({ ...draft, columns }, CATALOG)
        expect(JSON.stringify(saved.display?.columns)).toBe(
            JSON.stringify({ customer: { minWidth: '7.5ch', whiteSpace: 'normal', lineClamp: 2 } })
        )
    })

    it('writes no display when nothing is set, and keeps an empty one the spec spelled out', () => {
        const plain = { ...FIT_SPEC, display: undefined }
        expect(specFromDraft(draftFromSpec(plain, META, CATALOG), CATALOG).display).toBeUndefined()
        const spelled = { ...FIT_SPEC, display: {} }
        expect(specFromDraft(draftFromSpec(spelled, META, CATALOG), CATALOG).display).toEqual({})
    })

    it('tidies a fit: engine order, no unset setting, undefined when empty', () => {
        expect(tidyFit({ flexGrow: 2, width: '10ch', minWidth: undefined })).toEqual({
            width: '10ch',
            flexGrow: 2,
        })
        expect(JSON.stringify(tidyFit({ flexGrow: 2, width: '10ch' }))).toBe(
            '{"width":"10ch","flexGrow":2}'
        )
        expect(tidyFit({})).toBeUndefined()
        expect(tidyFit(undefined)).toBeUndefined()
    })
})
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/spec.test.ts core/reports/lib/builder/__tests__/draft.test.ts`
Expected: FAIL — `parseFitLength` and `tidyFit` are not exported; `parseSpec(FIT)` is `null` (strict `display` refuses `oneLine`).

- [ ] **Step 3: Implement the spec schema**

In `core/reports/lib/spec.ts`, replace `displayLayoutSchema` (`:187-193`) with:

```ts
// A column fit length (core/server/reports/fit.go fitLength): a number of 1 to 4
// digits, with decimals, in ch (the width of "0"), px or % (of the line).
export const FIT_LENGTH = /^(\d{1,4}(?:\.\d+)?)(ch|px|%)$/
const PX_LENGTH = /^\d{1,4}(?:\.\d+)?px$/
export type FitLength = { value: number; unit: 'ch' | 'px' | '%' }

const isFitUnit = (unit: string | undefined): unit is FitLength['unit'] =>
    unit === 'ch' || unit === 'px' || unit === '%'

export function parseFitLength(s: string): FitLength | null {
    const [, digits, unit] = FIT_LENGTH.exec(s) ?? []
    if (!digits || !isFitUnit(unit)) return null
    return { value: Number(digits), unit }
}

const fitLength = z.string().regex(FIT_LENGTH)
const pxLength = z.string().regex(PX_LENGTH)

// One column's fit, after CSS (design §3.2). The engine refuses each setting that
// could cut a figure; flexGrow is free on every column.
export const columnFitSchema = z.strictObject({
    width: fitLength.optional(),
    minWidth: z.union([z.enum(['auto', 'min-content', 'max-content']), fitLength]).optional(),
    maxWidth: z.union([z.literal('none'), fitLength]).optional(),
    whiteSpace: z.enum(['nowrap', 'normal']).optional(),
    lineClamp: z.number().int().min(1).max(4).optional(),
    textOverflow: z.literal('ellipsis').optional(),
    flexGrow: z.number().int().min(0).max(10).optional(),
})
export type ColumnFit = z.infer<typeof columnFitSchema>

// The least row width at which the report is drawn on one line.
export const displayOneLineSchema = z.strictObject({ minWidth: pxLength })
export type DisplayOneLine = z.infer<typeof displayOneLineSchema>

// The columns placed on lines of `tracks` equal tracks at one width; a column it
// does not list is not shown there. `minWidth`: the least row width it is used at.
export const displayLayoutSchema = z.strictObject({
    width: z.enum(DISPLAY_WIDTHS),
    minWidth: pxLength.optional(),
    tracks: z.number().int(),
    cells: z.array(displayCellSchema),
})
export type DisplayLayout = z.infer<typeof displayLayoutSchema>

/** The fit settings the table reads besides the arrangements. */
export type DisplayFit = { oneLine?: DisplayOneLine; columns?: Record<string, ColumnFit> }
```

Replace the `display` line of `reportSpecSchema` (`:226`) with:

```ts
    display: z
        .strictObject({
            oneLine: displayOneLineSchema.optional(),
            columns: z.record(z.string(), columnFitSchema).optional(),
            layouts: z.array(displayLayoutSchema).optional(),
        })
        .nullish(),
```

- [ ] **Step 4: Implement the draft**

In `core/reports/lib/builder/draft.ts`:

1. Import `type ColumnFit`, `columnFitSchema`, `type DisplayOneLine`, `displayOneLineSchema` from `'../spec'` (keep the list sorted as biome wants).
2. In `columnFields` (`:138-156`), after `rollup`, add:
   ```ts
       /** The column's fit (display.columns), kept as the spec says it; a save tidies it (tidyFit). */
       fit: columnFitSchema.optional(),
   ```
3. Replace `draftShape.display` (`:239-240`) with:
   ```ts
       /** The one-line width gate and the line layouts per width (E14), kept as the spec says them. */
       display: z.object({
           oneLine: displayOneLineSchema.optional(),
           layouts: z.array(displayLayoutSchema).max(DISPLAY_WIDTHS.length),
       }),
   ```
   (`emptyDraft`'s `display: { layouts: [] }` stays valid.)
4. `columnDraft` (`:622`): add a fifth parameter `fit: ColumnFit | undefined` and, in `common`, after the `rollup` spread: `...(fit ? { fit } : {}),`. At the call (`:693`):
   ```ts
       const columns = spec.columns.map(column =>
           columnDraft(column, catalog, spec.source, params, spec.display?.columns?.[column.id])
       )
   ```
5. `draftFromSpec` display (`:716`):
   ```ts
           display: {
               ...(spec.display?.oneLine ? { oneLine: spec.display.oneLine } : {}),
               layouts: spec.display?.layouts ?? [],
           },
   ```
6. Above `specFromDraft`, add:
   ```ts
   /** A column's fit in the engine's field order with no unset setting; undefined when nothing is set. */
   export function tidyFit(fit: ColumnFit | undefined): ColumnFit | undefined {
       if (!fit) return undefined
       const tidy: ColumnFit = {
           ...(fit.width !== undefined ? { width: fit.width } : {}),
           ...(fit.minWidth !== undefined ? { minWidth: fit.minWidth } : {}),
           ...(fit.maxWidth !== undefined ? { maxWidth: fit.maxWidth } : {}),
           ...(fit.whiteSpace !== undefined ? { whiteSpace: fit.whiteSpace } : {}),
           ...(fit.lineClamp !== undefined ? { lineClamp: fit.lineClamp } : {}),
           ...(fit.textOverflow !== undefined ? { textOverflow: fit.textOverflow } : {}),
           ...(fit.flexGrow !== undefined ? { flexGrow: fit.flexGrow } : {}),
       }
       return Object.keys(tidy).length > 0 ? tidy : undefined
   }

   // Go writes a map's keys in byte order (not locale order).
   function byteOrder(a: string, b: string): number {
       if (a < b) return -1
       return a > b ? 1 : 0
   }

   // The display in the engine's field order (oneLine, columns, layouts); an empty
   // display the read spec spelled out is kept, any other empty one is left out.
   function specDisplay(draft: BuilderDraft): ReportSpec['display'] {
       const fits = draft.columns
           .flatMap(column => {
               const fit = tidyFit(column.fit)
               return fit ? [[column.id, fit] as const] : []
           })
           .sort(([a], [b]) => byteOrder(a, b))
       const { oneLine, layouts } = draft.display
       const display = {
           ...(oneLine ? { oneLine } : {}),
           ...(fits.length > 0 ? { columns: Object.fromEntries(fits) } : {}),
           ...(layouts.length > 0 ? { layouts } : {}),
       }
       if (Object.keys(display).length > 0) return display
       return draft.explicit.display ? {} : undefined
   }
   ```
7. In `specFromDraft`, replace `:1026-1027` with:
   ```ts
       const display = specDisplay(draft)
       if (display) spec.display = display
   ```

`DisplayOneLine` is imported for the `BuilderDraft['display']` type only if the compiler needs it; drop the import if biome reports it unused.

- [ ] **Step 5: Keep `oneLine` when an arrangement changes**

In `core/reports/components/builder/ArrangeSection.tsx` `useArrange`, spread the display so a layout change does not drop the one-line width:

```ts
    const setLayout = (next: DisplayLayout) =>
        builder.setDisplay({
            ...display,
            layouts:
                index < 0
                    ? [...display.layouts, next]
                    : display.layouts.map((l, i) => (i === index ? next : l)),
        })
```

and in `onSwitch` (`:93`): `builder.setDisplay({ ...display, layouts: display.layouts.filter((_, i) => i !== index) })`. (The e2e in Task 12 sets "One line from" and then an arrangement, and checks both apply.)

- [ ] **Step 6: Run the tests to see them pass**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/spec.test.ts core/reports/lib/builder/__tests__/draft.test.ts core/reports/lib/builder/__tests__/errors.test.ts`
Expected: PASS (the earlier round-trip tests too: a spec with no fit writes the same display as before).

- [ ] **Step 7: Docs**

In `docs/packages.md`:

1. `:1321-1322`: replace "**Spec version 2.** Version 2 added these fields. A version 2 spec sent to an older server is refused by its version." with "**Fields added in version 2.**".
2. `:1339` (`display.layouts[]` row): change the shape to `{ width, minWidth?, tracks, cells: [...] }`; change "The view uses one line when its columns fit" to "The view uses one line when the rows are at least `display.oneLine.minWidth` wide and its columns fit", and "else the first arrangement whose lines fit" to "else the first arrangement the rows reach (its `minWidth`) whose lines fit"; and add at the end: "`minWidth` (px, for example `\"600px\"`): the arrangement is used only where the rows are at least that wide."
3. `:1341-1344`: replace the "**Spec version 3.** …" paragraph with "**Fields added in version 3.**".
4. After the version 3 table, add:

   ```markdown
   **Spec version 4.** `reports.CurrentVersion` is 4, and it is the only version a
   server reads: a spec of any other version is refused (`version must be 4`), and a
   saved report of another version opens Broken. Version 4 adds the column fit, read
   as the CSS properties of the same names. A setting that could cut a figure (a
   number, amount, percent, date, datetime or yes/no column, or a first column whose
   label is one) is refused: a figure takes only `flexGrow` and `minWidth` `auto` or
   `max-content`.

   | Field | What it does |
   |---|---|
   | `display.oneLine.minWidth` | px. The rows are drawn on one line only where they are at least this wide (and the columns fit); below it the arrangements and automatic lines are used. |
   | `display.columns.<col>.width` | A length (`12ch`, `120px`, `25%`): the column is exactly this wide. `ch` and `px` are the text's width (the cell's padding is added); `%` is the cell's whole width as a part of the line. |
   | `display.columns.<col>.minWidth` | `auto` (default: eight letters for text, the whole value for a figure), `min-content` (the longest word), `max-content` (the whole value), or a length. |
   | `display.columns.<col>.maxWidth` | `none` (default) or a length; never under `minWidth`. |
   | `display.columns.<col>.whiteSpace` | `nowrap` (default: one line, ends in "…") or `normal` (the text wraps). |
   | `display.columns.<col>.lineClamp` | 1 to 4, with `whiteSpace: "normal"`: the most lines the text wraps to (default 2). |
   | `display.columns.<col>.textOverflow` | `ellipsis` only. |
   | `display.columns.<col>.flexGrow` | 0 to 10: the column's share of spare room on one line (default 1 for text, 0 for figures). |
   ```
5. `:1389`: `"version": 2` → `"version": 4`.

- [ ] **Step 8: Typecheck and lint**

Run: `pnpm exec tinycld-pkg typecheck` then `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check core/reports/lib/spec.ts core/reports/lib/builder/draft.ts core/reports/components/builder/ArrangeSection.tsx core/reports/lib/__tests__/spec.test.ts core/reports/lib/builder/__tests__/draft.test.ts`
Expected: no errors.

- [ ] **Step 9: Stage**

`core/reports/lib/spec.ts core/reports/lib/builder/draft.ts core/reports/components/builder/ArrangeSection.tsx core/reports/lib/__tests__/spec.test.ts core/reports/lib/builder/__tests__/draft.test.ts docs/packages.md`

---

### Task 9: The measure resolves column fit

**Files:**
- Modify: `core/reports/lib/column-measure.ts` (`ColumnMeasure` `:15`, `FIGURE_TYPES`/`isFigureColumn` `:41-54`, `widestCell` `:155-167`, `labelWidths` `:178-197`, `measureKey` `:204-213`, `measureColumns` `:215-244`)
- Test: `core/reports/lib/__tests__/column-measure.test.ts`

**Interfaces:**
- Consumes: Task 8 `ColumnFit`, `parseFitLength`, `FitLength`; `textWidth`, `longestWordWidth` (`text-width.ts:55,64`); `labelChrome`, `labelNeed`, `CELL_PADDING`, `TEXT_MIN_WIDTH`.
- Produces: `type ColumnMeasure = { natural: number; min: number; grow: number; max?: number; lines?: number; pct?: { width?: number; min?: number; max?: number } }` (`pct` values are fractions 0–1); `CH_WIDTH: number`; `DEFAULT_LINE_CLAMP = 2`; `isFigureType(type: FieldType | undefined): boolean`; `measureColumns({ columns, rows, format, levels, fits?: Readonly<Record<string, ColumnFit>> })`; `measureKey({ runKey, format, columns, levels, fits? })`.

- [ ] **Step 1: Write the failing tests**

Append to `core/reports/lib/__tests__/column-measure.test.ts` (add `CH_WIDTH` and `isFigureType` to the `'../column-measure'` import, and `import type { ColumnFit } from '../spec'`):

```ts
describe('column fit in the measure', () => {
    // An untyped label, so the label column is text and takes settings.
    const NOTE = item('detail', 'Office supplies for the front desk', {
        number: 'JE-7',
        memo: 'Reversal of JE-12 for October rent',
        debit: 1200,
    })
    const rows = [GROUP, NOTE, TOTAL]
    const plain = measureColumns({ columns: COLUMNS, rows, format: DEFAULT_DISPLAY, levels: GROUPED })
    const withFits = (fits: Record<string, ColumnFit>) =>
        measureColumns({ columns: COLUMNS, rows, format: DEFAULT_DISPLAY, levels: GROUPED, fits })
    const ch = (n: number) => Math.ceil(n * CH_WIDTH) + CELL_PADDING
    const memo = plain.memo ?? { natural: 0, min: 0, grow: 0 }

    it('changes nothing without settings', () => {
        expect(withFits({})).toEqual(plain)
    })

    it('raises a text column to its minWidth, and its natural width with it', () => {
        const wide = withFits({ memo: { minWidth: '60ch' } }).memo
        expect(wide).toEqual({ natural: Math.max(memo.natural, ch(60)), min: ch(60), grow: 1 })
    })

    it('keeps the whole value with max-content and the longest word with min-content', () => {
        const m = withFits({ memo: { minWidth: 'max-content' }, number: { minWidth: 'min-content' } })
        expect(m.memo?.min).toBe(memo.natural)
        expect(m.number?.min).toBe(
            Math.max(
                longestWordWidth('No.', HEADING) + CELL_PADDING,
                longestWordWidth('JE-7', CELL) + CELL_PADDING
            )
        )
    })

    it('caps a column at maxWidth, never under its min', () => {
        const m = withFits({ memo: { maxWidth: '10ch' } }).memo
        expect(m?.max).toBe(Math.max(memo.min, ch(10)))
        expect(m?.natural).toBe(m?.max)
    })

    it('fixes a column at width', () => {
        expect(withFits({ memo: { width: '100px' } }).memo).toEqual({
            natural: 100 + CELL_PADDING,
            min: 100 + CELL_PADDING,
            max: 100 + CELL_PADDING,
            grow: 1,
        })
    })

    it('lets a wrapping column narrow to its longest word or eight letters, on two lines', () => {
        const m = withFits({ memo: { whiteSpace: 'normal' } }).memo
        const word = longestWordWidth('Reversal of JE-12 for October rent', CELL) + CELL_PADDING
        expect(m?.lines).toBe(2)
        expect(m?.min).toBe(
            Math.max(memo.min, Math.min(memo.natural, Math.max(word, TEXT_MIN_WIDTH + CELL_PADDING)))
        )
        expect(withFits({ memo: { whiteSpace: 'normal', lineClamp: 3 } }).memo?.lines).toBe(3)
    })

    it('keeps a % for the fit to resolve at the line width', () => {
        expect(withFits({ memo: { width: '25%' } }).memo).toEqual({ ...memo, pct: { width: 0.25 } })
        expect(withFits({ memo: { minWidth: '10%', maxWidth: '40%' } }).memo?.pct).toEqual({
            min: 0.1,
            max: 0.4,
        })
    })

    it('takes flexGrow on a text column and on a figure', () => {
        expect(withFits({ memo: { flexGrow: 3 } }).memo).toEqual({ ...memo, grow: 3 })
        expect(withFits({ debit: { flexGrow: 2 } }).debit).toEqual({ ...plain.debit, grow: 2 })
    })

    it('ignores a setting that would cut a figure (the engine refuses it)', () => {
        expect(withFits({ debit: { width: '2ch', whiteSpace: 'normal' } }).debit).toEqual(plain.debit)
    })

    it('pads a label length with the label chrome', () => {
        expect(withFits({ account: { width: '20ch' } }).account?.min).toBe(
            Math.ceil(20 * CH_WIDTH) + labelChrome(GROUPED)
        )
    })

    it('never cuts a typed label, whatever its setting', () => {
        const typed = [GROUP, DETAIL, TOTAL]
        const base = measureColumns({ columns: COLUMNS, rows: typed, format: DEFAULT_DISPLAY, levels: GROUPED })
        const set = measureColumns({
            columns: COLUMNS,
            rows: typed,
            format: DEFAULT_DISPLAY,
            levels: GROUPED,
            fits: { account: { width: '2ch' } },
        })
        expect(set.account).toEqual(base.account)
    })

    it('never cuts a label column of figures, even when its rows carry no label type', () => {
        const columns: ResultColumn[] = [
            { id: 'amount', label: 'Amt', type: 'money', currency: 'USD' },
            { id: 'memo', label: 'Memo', type: 'text' },
        ]
        const row = item('detail', '$12,345.67 paid in full on account', { memo: 'x' })
        const m = measureColumns({ columns, rows: [row], format: DEFAULT_DISPLAY, levels: FLAT })
        expect(m.amount?.min).toBe(labelNeed(row, DEFAULT_DISPLAY))
        expect(isFigureType('money')).toBe(true)
        expect(isFigureType('text')).toBe(false)
    })
})

describe('measureKey with fits', () => {
    it('changes with a column fit', () => {
        const base = { runKey: 'r1', format: DEFAULT_DISPLAY, columns: COLUMNS, levels: GROUPED }
        expect(measureKey({ ...base, fits: { memo: { minWidth: '12ch' } } })).not.toBe(measureKey(base))
        expect(measureKey({ ...base, fits: {} })).toBe(measureKey(base))
    })
})
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/column-measure.test.ts`
Expected: FAIL — `CH_WIDTH` / `isFigureType` not exported; `fits` ignored (the minWidth case gets the plain measure); the figure-label case gets the text floor.

- [ ] **Step 3: Implement**

In `core/reports/lib/column-measure.ts`:

1. Imports: `import { type ColumnFit, type FitLength, parseFitLength } from './spec'` (merge with the existing `type SpecColumn` import).
2. Replace `ColumnMeasure` (`:15`):
   ```ts
   /**
    * A column's widths: what its content needs, the least it may get, its share of
    * spare room; and from its fit (display.columns) the most it may get, the lines its
    * text may wrap to, and the parts of the line width a % setting asks for (0–1),
    * which the fit resolves at each line's width (table-lines.ts atWidth).
    */
   export type ColumnMeasure = {
       natural: number
       min: number
       grow: number
       max?: number
       lines?: number
       pct?: { width?: number; min?: number; max?: number }
   }
   ```
3. After `TEXT_MIN_WIDTH`, add:
   ```ts
   /** The width of "0" in a cell: a ch. */
   export const CH_WIDTH = textWidth('0', CELL)
   /** The lines a wrapping column's text may take when its fit gives no lineClamp. */
   export const DEFAULT_LINE_CLAMP = 2
   ```
4. Replace `isFigureColumn` (`:50-54`) with:
   ```ts
   export function isFigureType(type: FieldType | undefined): boolean {
       return type !== undefined && FIGURE_TYPES.has(type)
   }

   export function isFigureColumn(column: ResultColumn): boolean {
       return isFigureType(column.type) || isFigureType(column.totalType)
   }
   ```
5. `widestCell` returns `{ widest: number; longestWord: number }`: in the loop also keep `longestWord = Math.max(longestWord, longestWordWidth(text, cellTextStyle(item.row.kind)))`; return `{ widest: widest === 0 ? 0 : widest + CELL_PADDING, longestWord: longestWord === 0 ? 0 : longestWord + CELL_PADDING }`.
6. `labelWidths` also returns `longestWord`: for each row it counts, `longestWord = Math.max(longestWord, labelWordNeed(item, format))`, with
   ```ts
   // The width a row's label needs for its longest word, as labelNeed for the whole label.
   function labelWordNeed(item: VisibleRow, format: NumberFormat): number {
       const { row } = item
       const start = INDENT + row.depth * INDENT + (item.isToggle ? CARET_ROOM : 0)
       const word = longestWordWidth(rowLabelText(row, format), cellTextStyle(row.kind))
       return start + word + CELL_PADDING / 2
   }
   ```
7. Add the fit resolution above `measureKey`:
   ```ts
   // What a column's content asks for before its fit. Widths include the padding
   // (`pad`: a cell's px-3 on both sides, or the label's chrome); `floor` is the
   // automatic least width (eight letters, or the whole value of a typed label).
   type Content = {
       natural: number
       headingMin: number
       floor: number
       minContent: number
       pad: number
   }

   type Pct = NonNullable<ColumnMeasure['pct']>

   const autoMin = (c: Content) => Math.max(c.headingMin, Math.min(c.natural, c.floor))
   const pctOf = (length: FitLength) => length.value / 100

   // A ch or px length is the text's width, so the padding is added; % is resolved later.
   function lengthPx(length: FitLength, pad: number): number {
       const text = length.unit === 'ch' ? length.value * CH_WIDTH : length.value
       return Math.ceil(text) + pad
   }

   function applyWidth(out: ColumnMeasure, c: Content, value: string | undefined, pct: Pct) {
       const length = value ? parseFitLength(value) : null
       if (!length) return
       if (length.unit === '%') {
           pct.width = pctOf(length)
           return
       }
       const px = Math.max(c.headingMin, lengthPx(length, c.pad))
       out.natural = px
       out.min = px
       out.max = px
   }

   function applyMin(out: ColumnMeasure, c: Content, value: string | undefined, pct: Pct) {
       if (value === 'min-content') out.min = Math.max(c.headingMin, c.minContent)
       else if (value === 'max-content') out.min = c.natural
       else if (value && value !== 'auto') {
           const length = parseFitLength(value)
           if (length?.unit === '%') pct.min = pctOf(length)
           else if (length) out.min = Math.max(c.headingMin, lengthPx(length, c.pad))
       }
       out.natural = Math.max(out.natural, out.min)
       if (out.max !== undefined) out.max = Math.max(out.max, out.min)
   }

   function applyMax(out: ColumnMeasure, c: Content, value: string | undefined, pct: Pct) {
       const length = value && value !== 'none' ? parseFitLength(value) : null
       if (!length) return
       if (length.unit === '%') {
           pct.max = pctOf(length)
           return
       }
       const cap = Math.max(out.min, lengthPx(length, c.pad))
       out.max = out.max === undefined ? cap : Math.min(out.max, cap)
       out.natural = Math.min(out.natural, out.max)
   }

   // A text column's measure under its fit, read as CSS (design §3.2): `width` sets
   // it, `minWidth` raises its least width (never under the heading's longest word),
   // `maxWidth` caps it but never under its least. A wrap may narrow it to its
   // longest word (or eight letters), never cutting a word.
   function fitted(c: Content, fit: ColumnFit | undefined, grow: number): ColumnMeasure {
       const out: ColumnMeasure = { natural: c.natural, min: autoMin(c), grow: fit?.flexGrow ?? grow }
       if (!fit) return out
       const pct: Pct = {}
       if (fit.whiteSpace === 'normal') {
           out.lines = fit.lineClamp ?? DEFAULT_LINE_CLAMP
           out.min = Math.max(c.headingMin, Math.min(c.natural, Math.max(c.minContent, c.floor)))
       }
       applyWidth(out, c, fit.width, pct)
       applyMin(out, c, fit.minWidth, pct)
       applyMax(out, c, fit.maxWidth, pct)
       if (Object.keys(pct).length > 0) out.pct = pct
       return out
   }

   // A figure is never cut (decision 2): its fit gives only its share of spare room.
   const wholeMeasure = (c: Content, fit: ColumnFit | undefined, grow: number): ColumnMeasure => ({
       natural: c.natural,
       min: autoMin(c),
       grow: fit?.flexGrow ?? grow,
   })

   const NO_FITS: Readonly<Record<string, ColumnFit>> = {}
   ```
   Note `applyMin` with `value === undefined` still runs `out.natural = Math.max(out.natural, out.min)`: after a wrap or a width, natural is already ≥ min, so this changes nothing in the default path; `fitted` without `fit` returns before it, so the plain measure has exactly the keys `natural`, `min`, `grow`.
8. `measureKey`: add `fits?: Readonly<Record<string, ColumnFit>>` to the input; return `JSON.stringify([runKey, format, shape, levels, input.fits ?? NO_FITS])`.
9. Replace the body of `measureColumns` after `const [label, ...others] = columns`:
   ```ts
       const fits = input.fits ?? NO_FITS
       const measures: ColumnMeasures = {}
       if (label) {
           const heading = headingWidths(label, format)
           const labels = labelWidths(others, rows, format)
           // A label of figures (a typed label, or a figure column first) is never cut.
           const isWhole = labels.isTyped || isFigureColumn(label)
           const chrome = labelChrome(levels)
           const content: Content = {
               natural: Math.max(heading.natural, labels.widest),
               headingMin: heading.min,
               floor: isWhole ? labels.widest : chrome + TEXT_MIN_WIDTH,
               minContent: labels.longestWord,
               pad: chrome,
           }
           const fit = fits[label.id]
           measures[label.id] = isWhole ? wholeMeasure(content, fit, 1) : fitted(content, fit, 1)
       }
       for (const column of others) {
           const heading = headingWidths(column, format)
           const cells = widestCell(column, rows, format)
           const natural = Math.max(heading.natural, cells.widest)
           const fit = fits[column.id]
           if (isFigureColumn(column)) {
               const whole = { natural, headingMin: heading.min, floor: cells.widest, minContent: 0, pad: 0 }
               measures[column.id] = wholeMeasure(whole, fit, 0)
               continue
           }
           const content: Content = {
               natural,
               headingMin: heading.min,
               floor: TEXT_MIN_WIDTH + CELL_PADDING,
               minContent: cells.longestWord,
               pad: CELL_PADDING,
           }
           measures[column.id] = fitted(content, fit, 1)
       }
       return measures
   ```
   Check against the old rules: a figure's `autoMin` is `max(heading.min, min(natural, widest)) = max(heading.min, widest)` (natural ≥ widest); a text column's is `max(heading.min, min(natural, TEXT_MIN_WIDTH + CELL_PADDING))`; a label's is `max(heading.min, min(natural, floor))` — the same as before for text and typed labels. Add `fits` to `measureColumns`' input type with the doc comment "The report's column fit settings (display.columns); absent: the defaults."
10. Update the comment at `:9-12` to say a column's width comes from its content and its fit settings.

- [ ] **Step 4: Run the tests to see them pass**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/column-measure.test.ts core/reports/lib/__tests__/table-lines.test.ts`
Expected: PASS (the earlier measure and table-lines cases unchanged).

- [ ] **Step 5: Typecheck and lint**

Run: `pnpm exec tinycld-pkg typecheck` then `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check core/reports/lib/column-measure.ts core/reports/lib/__tests__/column-measure.test.ts`
Expected: no errors.

- [ ] **Step 6: Stage**

`core/reports/lib/column-measure.ts core/reports/lib/__tests__/column-measure.test.ts`

---

### Task 10: The fit reads the settings

**Files:**
- Modify: `core/reports/lib/table-lines.ts` (`LineLayout` `:43-45`, `autoLines` `:220-255`, `shareWidth` `:334-353`, `extentOf` `:373-389`, `fitRoot` `:447-467`, `tableLines` `:479-509`, `rowLayout` `:613-614`)
- Test: `core/reports/lib/__tests__/table-lines.test.ts`

**Interfaces:**
- Consumes: Task 9 `ColumnMeasure` (`max`, `lines`, `pct`); Task 8 `DisplayOneLine`, `parseFitLength`, `DisplayLayout.minWidth`; Part 1 `ARRANGEMENT_NOTICE`.
- Produces: `atWidth(m: ColumnMeasure, lineWidth: number): ColumnMeasure`; `shareWidth` caps grow at `max`; `type LayoutSource = 'one-line' | DisplayWidth | 'auto'`; `type FitTableInput = { columns; width; layouts; levels?; measures?; oneLine?: DisplayOneLine }`; `fitTable(input: FitTableInput): { layout: LineLayout; source: LayoutSource }`; `tableLines(input: FitTableInput): LineLayout` (= `fitTable(input).layout`); `LineLayout` variants gain `lines?: Record<string, number>`.

- [ ] **Step 1: Write the failing tests**

Append to `core/reports/lib/__tests__/table-lines.test.ts` (add `atWidth` and `fitTable` to the `'../table-lines'` import, and `type ColumnMeasure` to the `'../column-measure'` import):

```ts
describe('column fit in the fit', () => {
    const FIT_MEASURES: ColumnMeasures = {
        account: m(120, 180, 1),
        number: m(64, 64, 1),
        debit: m(110, 110, 0),
        credit: m(110, 110, 0),
    }
    // Mins: 120 + 64 + 110 + 110 = 404.
    const fit = (width: number, extra: Partial<Parameters<typeof fitTable>[0]> = {}) =>
        fitTable({ columns: COLS, width, layouts: [], measures: FIT_MEASURES, ...extra })
    const TABLET: DisplayLayout = {
        width: 'tablet',
        tracks: 2,
        cells: [
            { col: 'account', line: 1, span: 2 },
            { col: 'number', line: 2 },
            { col: 'debit', line: 2 },
            { col: 'credit', line: 3 },
        ],
    }

    it('stops a grower at its max and shares the rest among the others', () => {
        const capped: ColumnMeasure = { min: 80, natural: 120, grow: 1, max: 150 }
        expect(shareWidth([m(100, 100, 1), capped], 600)).toEqual([450, 150])
        expect(
            shareWidth([m(100, 100, 1), { min: 100, natural: 100, grow: 1, max: 120 }, m(100, 100, 1)], 600)
        ).toEqual([240, 120, 240])
    })

    it('resolves a % at the line width', () => {
        expect(atWidth({ natural: 100, min: 50, grow: 1, pct: { width: 0.25 } }, 800)).toEqual({
            natural: 200,
            min: 200,
            max: 200,
            grow: 1,
        })
        expect(atWidth({ natural: 300, min: 50, grow: 1, pct: { min: 0.1, max: 0.2 } }, 1000)).toEqual({
            natural: 200,
            min: 100,
            max: 200,
            grow: 1,
        })
        const plain = m(50, 100, 1)
        expect(atWidth(plain, 1000)).toBe(plain)
    })

    it('a % width is a part of the line', () => {
        const measures = { ...FIT_MEASURES, number: { ...m(64, 64, 1), pct: { width: 0.25 } } }
        expect(fit(800, { measures }).layout).toEqual({
            kind: 'one-line',
            widths: { account: 380, number: 200, debit: 110, credit: 110 },
        })
    })

    it('draws one line only from the one-line width', () => {
        expect(fit(500, { oneLine: { minWidth: '600px' } }).source).toBe('auto')
        expect(fit(600, { oneLine: { minWidth: '600px' } }).source).toBe('one-line')
        expect(fit(500).source).toBe('one-line')
    })

    it('never one line under the gate, however wide the columns fit', () => {
        const wide = fit(1280, { oneLine: { minWidth: '4000px' } })
        expect(wide.source).toBe('auto')
        expect(wide.layout.kind).toBe('blocks')
    })

    it('uses an arrangement only from its minWidth, and says nothing when it skips it', () => {
        expect(fit(380, { layouts: [{ ...TABLET, minWidth: '350px' }] }).source).toBe('tablet')
        const skipped = fit(380, { layouts: [{ ...TABLET, minWidth: '390px' }] })
        expect(skipped.source).toBe('auto')
        expect(skipped.layout.kind === 'blocks' && skipped.layout.notice).toBeFalsy()
    })

    it('carries the lines a wrapping column may take, on every row', () => {
        const measures = { ...FIT_MEASURES, account: { ...m(120, 180, 1), lines: 2 } }
        expect(fit(1280, { measures }).layout).toMatchObject({ kind: 'one-line', lines: { account: 2 } })
        const auto = fit(300, { measures }).layout
        expect(auto).toMatchObject({ kind: 'blocks', lines: { account: 2 } })
        const group: VisibleRow = {
            row: { id: 'group:A', kind: 'group', depth: 0, label: 'A', cells: {} },
            cells: {},
            isToggle: true,
            isOpen: false,
        }
        expect(rowLayout(group, auto, 'account', 0)).toMatchObject({ lines: { account: 2 } })
        expect('lines' in fit(1280).layout).toBe(false)
    })

    it('says which rule laid the rows out', () => {
        expect(fit(1280).source).toBe('one-line')
        expect(fit(300).source).toBe('auto')
        expect(tableLines({ columns: COLS, width: 1280, layouts: [], measures: FIT_MEASURES })).toEqual(
            fit(1280).layout
        )
    })
})
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/table-lines.test.ts`
Expected: FAIL — `atWidth` and `fitTable` are not exported.

- [ ] **Step 3: Implement**

In `core/reports/lib/table-lines.ts`:

1. Imports: add `parseFitLength`, `type DisplayOneLine` to the `'./spec'` import.
2. `LineLayout` (`:43-45`):
   ```ts
   /**
    * One line, each column at its width; or blocks whose side parts carry px widths.
    * `lines`: the columns whose text wraps, and to how many lines (absent: one).
    */
   export type LineLayout =
       | { kind: 'one-line'; widths: Record<string, number>; lines?: Record<string, number> }
       | {
             kind: 'blocks'
             root: Block
             indent: number
             notice?: string
             lines?: Record<string, number>
         }
   /** Which rule laid the rows out: one line, an author arrangement, or automatic lines. */
   export type LayoutSource = 'one-line' | DisplayWidth | 'auto'
   ```
3. After `measureOf`, add:
   ```ts
   /** A column's measure at a line's width: its % settings turned into px. */
   export function atWidth(m: ColumnMeasure, lineWidth: number): ColumnMeasure {
       const { pct, ...rest } = m
       if (!pct) return m
       const part = (share: number) => Math.floor(share * lineWidth)
       if (pct.width !== undefined) {
           // A fixed share of the line, never under the column's least width.
           const px = Math.max(rest.min, part(pct.width))
           return { ...rest, natural: px, min: px, max: px }
       }
       const min = pct.min === undefined ? rest.min : Math.max(rest.min, part(pct.min))
       const cap = pct.max === undefined ? rest.max : Math.min(rest.max ?? Infinity, part(pct.max))
       const max = cap === undefined ? undefined : Math.max(min, cap)
       const natural = Math.min(Math.max(rest.natural, min), max ?? Infinity)
       return { ...rest, natural, min, ...(max === undefined ? {} : { max }) }
   }

   // The measures at a line's width; the same object when no column has a % setting.
   function measuresAt(measures: ColumnMeasures, lineWidth: number): ColumnMeasures {
       if (!Object.values(measures).some(m => m.pct)) return measures
       return Object.fromEntries(
           Object.entries(measures).map(([id, m]) => [id, atWidth(m, lineWidth)])
       )
   }
   ```
4. `shareWidth`: keep the min and lack steps; replace the grow step (`:345-350`) with a call `growInto(widths, parts, width)` and add above it:
   ```ts
   // The spare room by grow, in rounds: a part that reaches its max keeps it, and the
   // next round shares what is left among the others. With no max it is one round.
   // When every grower is capped, shareWidth still gives the rest to the first part
   // (the label), past its max: a row is always filled.
   function growInto(widths: number[], parts: readonly ColumnMeasure[], width: number) {
       const capped = new Set<number>()
       for (let round = 0; round < parts.length; round++) {
           const spare = width - sum(widths)
           const open = parts.flatMap((part, i) => (part.grow > 0 && !capped.has(i) ? [i] : []))
           const totalGrow = sum(open.map(i => parts[i]?.grow ?? 0))
           if (spare <= 0 || totalGrow === 0) return
           let isCapped = false
           for (const i of open) {
               const part = parts[i]
               if (!part) continue
               const room = (part.max ?? Infinity) - (widths[i] ?? 0)
               const add = Math.floor((spare * part.grow) / totalGrow)
               widths[i] = (widths[i] ?? 0) + Math.min(add, room)
               if (add >= room) {
                   capped.add(i)
                   isCapped = true
               }
           }
           if (!isCapped) return
       }
   }
   ```
   The lack step never passes `max` (Task 9 keeps `natural ≤ max`). Update the doc comment: "…then by grow, a part stopping at its max."
5. `extentOf`: type `Extent = { min: number; natural: number; max?: number }`; a cell returns its measure's `min`, `natural` and `max` (a stack or side block has none), so a side part holding one cell is capped in `fitBlock`'s `shareWidth`.
6. `fitRoot`: resolve the measures per line width: in the non-stack branch `fitBlock(root, width, measuresAt(measures, width), clamp)`; in the loop `const lineWidth = i === 0 ? width : Math.max(0, width - indent)` and `fitBlock(part, lineWidth, measuresAt(measures, lineWidth), clamp)`.
7. `autoLines`: pack by the mins at the row width: `const at = measuresAt(measures, width)` and read `measureOf(at, column.id).min`; keep passing `measures` (not `at`) to `fitRoot`.
8. Replace `tableLines` (`:473-509`) with:
   ```ts
   export type FitTableInput = {
       columns: readonly ResultColumn[]
       width: number
       layouts: readonly DisplayLayout[]
       /** How deep the report's rows go (rowLevelsOf); absent: a flat report. */
       levels?: RowLevels
       /** The run's measure (measureColumns); absent: the headings and worst cases alone. */
       measures?: ColumnMeasures
       /** The least row width for one line (display.oneLine); absent: any width. */
       oneLine?: DisplayOneLine
   }

   // A px gate (display.oneLine.minWidth, layouts[].minWidth); 0 when unset.
   const gateOf = (minWidth: string | undefined) =>
       minWidth ? (parseFitLength(minWidth)?.value ?? 0) : 0

   function linesOf(measures: ColumnMeasures): Record<string, number> | undefined {
       const wrapping = Object.entries(measures).flatMap(([id, m]) =>
           m.lines !== undefined && m.lines > 1 ? [[id, m.lines] as const] : []
       )
       return wrapping.length > 0 ? Object.fromEntries(wrapping) : undefined
   }

   /**
    * The lines of every row (and the heading) at this width, by a fit test on the
    * measured width (review §7.1-4): one line when the row is at least the one-line
    * width and its columns' mins fit; else the first author arrangement, widest
    * first, that the width reaches (its minWidth) and whose lines fit, each line
    * shared by content; else the automatic lines. Never wider than `width`. Also
    * says which rule it used (the table's e2e marker).
    */
   export function fitTable(input: FitTableInput): { layout: LineLayout; source: LayoutSource } {
       const { columns, width, layouts } = input
       const levels = input.levels ?? FLAT_LEVELS
       const measures =
           input.measures ?? measureColumns({ columns, rows: [], format: DEFAULT_DISPLAY, levels })
       const lines = linesOf(measures)
       const withLines = <T extends LineLayout>(layout: T): T => (lines ? { ...layout, lines } : layout)
       const isOneLineWidth = width >= gateOf(input.oneLine?.minWidth)
       const widths = isOneLineWidth ? fitOneLine(columns, width, measuresAt(measures, width)) : null
       if (widths) return { layout: withLines({ kind: 'one-line', widths }), source: 'one-line' }
       const indent = continuationIndent(levels)
       let notice: string | undefined
       for (const name of WIDTH_ORDER) {
           const layout = layouts.find(l => l.width === name)
           // Below its minWidth an arrangement is not tried; it is not refused, so no notice.
           if (!layout || width < gateOf(layout.minWidth)) continue
           const authored = authoredLines(layout, columns)
           if (authored.kind === 'refused') {
               notice = notice ?? authored.notice
               continue
           }
           const root = fitRoot(authored.root, width, measures, indent)
           if (root) return { layout: withLines({ kind: 'blocks', root, indent }), source: name }
       }
       const auto = autoLines(columns, width, measures, indent)
       return { layout: withLines(notice ? { ...auto, notice } : auto), source: 'auto' }
   }

   export function tableLines(input: FitTableInput): LineLayout {
       return fitTable(input).layout
   }
   ```
9. `rowLayout` (`:613-614`): keep the wrap on a label-only row:
   ```ts
       if (Object.keys(item.cells).length === 0)
           return {
               kind: 'blocks',
               root: labelOnly,
               indent: layout.indent,
               ...(layout.lines ? { lines: layout.lines } : {}),
           }
   ```
   (The pruned branch spreads `layout`, so it keeps `lines`.)

- [ ] **Step 4: Run the tests to see them pass**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/table-lines.test.ts core/reports/lib/__tests__/column-measure.test.ts`
Expected: PASS, the earlier `shareWidth` and `tableLines` cases unchanged.

- [ ] **Step 5: Typecheck and lint**

Run: `pnpm exec tinycld-pkg typecheck` then `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check core/reports/lib/table-lines.ts core/reports/lib/__tests__/table-lines.test.ts`
Expected: no errors. (`useTableLayout` still calls `tableLines` with the old input, a subset of `FitTableInput`.)

- [ ] **Step 6: Stage**

`core/reports/lib/table-lines.ts core/reports/lib/__tests__/table-lines.test.ts`

---

### Task 11: Fit settings reach the view; the layout-settled marker

**Files:**
- Modify: `core/reports/lib/spec.ts` (add `displayFitOf`), test in `core/reports/lib/__tests__/spec.test.ts`
- Modify: `core/reports/hooks/useReportView.ts:301` area (return `fit`), `:346-348` (`ReportViewModel`)
- Modify: `core/reports/hooks/useTableLayout.ts` (whole file, 87 lines)
- Modify: `core/reports/components/ReportTable.tsx` `:530-562` (`ReportTableProps.fit`), `:573-605` (`dataSet`)
- Modify: `core/reports/components/ReportView.tsx:159-176` (`TableBody` passes `fit`)
- Create: `core/types/react-native-dataset.d.ts`; Modify: `core/package.json:48` (export), `core/tsconfig.package-base.json` (`types`)
- Modify: `tests/e2e/helpers.ts` (`waitForTableFit`), `tests/e2e/reports-fit.spec.ts:125-140`

Eleven files, each a few lines; the `.d.ts`, the export and the `types` entry are one change (members typecheck the `dataSet` prop through it), so the task is not split.

**Interfaces:**
- Consumes: Task 8 `DisplayFit`, `ReportSpec['display']`; Task 9 `measureColumns({ fits })`, `measureKey({ fits })`; Task 10 `fitTable`, `LayoutSource`.
- Produces: `displayFitOf(spec: Pick<ReportSpec, 'display'> | null | undefined): DisplayFit`; `ReportViewModel.fit?: DisplayFit`; `TableLayoutModel` picks `'fit'`; `TableLayout.fit: { width: number; source: LayoutSource }`; `ReportTableProps.fit?: { width: number; source: LayoutSource }`; DOM `data-fit-width` / `data-layout` on `[data-testid="report-table"]` once measured; `[data-testid="report-width-probe"]` (the row probe's existing test id, kept); `waitForTableFit(page: Page): Promise<string>` (`tests/e2e/helpers.ts`).

react-native-web 0.21 writes a `dataSet` prop as `data-*` attributes, hyphenating the keys (`createDOMProps` → `hyphenateString`): `fitWidth` → `data-fit-width`. React Native's `ViewProps` has no `dataSet`; the module augmentation below adds it (checked with tsc: `<View dataSet={{ a: '1' }} />` fails without it and passes with it). Native ignores the prop.

- [ ] **Step 1: Write the failing unit test**

Append to `core/reports/lib/__tests__/spec.test.ts` (add `displayFitOf` to the import):

```ts
describe('displayFitOf', () => {
    it('gives the one-line width and the column fits, and nothing else', () => {
        const display = {
            oneLine: { minWidth: '1040px' },
            columns: { note: { minWidth: '12ch' } },
            layouts: [{ width: 'mobile' as const, tracks: 1, cells: [{ col: 'note', line: 1 }] }],
        }
        expect(displayFitOf({ display })).toEqual({
            oneLine: { minWidth: '1040px' },
            columns: { note: { minWidth: '12ch' } },
        })
        expect(displayFitOf({ display: { layouts: display.layouts } })).toEqual({})
        expect(displayFitOf(null)).toEqual({})
    })
})
```

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/spec.test.ts`
Expected: FAIL — `displayFitOf` is not exported.

- [ ] **Step 2: Implement `displayFitOf`**

In `core/reports/lib/spec.ts`, after `parseSpec`:

```ts
/** The fit settings of a spec's display that the table reads besides its arrangements. */
export function displayFitOf(spec: Pick<ReportSpec, 'display'> | null | undefined): DisplayFit {
    const display = spec?.display
    return {
        ...(display?.oneLine ? { oneLine: display.oneLine } : {}),
        ...(display?.columns ? { columns: display.columns } : {}),
    }
}
```

Run the spec test again. Expected: PASS.

- [ ] **Step 3: The view model carries the fit**

In `core/reports/hooks/useReportView.ts`: import `displayFitOf` (and `type DisplayFit`) from `'../lib/spec'`; next to `layouts:` in the returned object add `fit: displayFitOf(input.spec),`. Replace the type at `:345-348`:

```ts
// onSetSort and fit are optional: a hand-built model (Books' mockup) has neither;
// then the Sort chip is not shown and the table takes the default fit.
export type ReportViewModel = Omit<ReturnType<typeof useReportView>, 'onSetSort' | 'fit'> & {
    onSetSort?(sort: ViewSort | null): void
    fit?: DisplayFit
}
```

- [ ] **Step 4: The layout reads the fit and says what it did**

Replace `core/reports/hooks/useTableLayout.ts` lines 1-19 and 45-87 so that:

```ts
import { useWindowWidth } from '@tinycld/core/components/workspace/useBreakpoint'
import { useMemo, useRef, useState } from 'react'
import { type ColumnMeasures, measureColumns, measureKey } from '../lib/column-measure'
import { type VisibleRow, visibleRows } from '../lib/rows'
import type { ColumnFit, DisplayFit } from '../lib/spec'
import { fitTable, type LayoutSource, type LineLayout } from '../lib/table-lines'
import type { ReportViewModel } from './useReportView'
import { useStable } from './useStable'

export type TableLayoutModel = Pick<
    ReportViewModel,
    'columns' | 'rows' | 'display' | 'layouts' | 'rowLevels' | 'run' | 'fit'
>

export interface TableLayout {
    layout: LineLayout
    /** False until the rows' width is measured: the table is drawn hidden until then. */
    isMeasured: boolean
    onRowWidth(width: number): void
    /** The width the layout was fitted to and the rule that laid it out (the table's e2e marker). */
    fit: { width: number; source: LayoutSource }
}

const NO_FIT: DisplayFit = {}
```

`useRunMeasures(model, fits: Readonly<Record<string, ColumnFit>> | undefined)` passes `fits` to both `measureKey({ …, fits })` and `measureColumns({ …, fits })` (a fit change is a new key, so the columns are measured again). `useTableLayout`:

```ts
export function useTableLayout(model: TableLayoutModel): TableLayout {
    const windowWidth = useWindowWidth()
    const [rowWidth, setRowWidth] = useState<number>()
    const fit = useStable(model.fit ?? NO_FIT)
    const measures = useRunMeasures(model, fit.columns)
    const columns = useStable(model.columns)
    const levels = useStable(model.rowLevels)
    const { layouts } = model
    const width = rowWidth ?? windowWidth
    const fitted = useMemo(
        () => fitTable({ columns, width, layouts, levels, measures, oneLine: fit.oneLine }),
        [columns, width, layouts, levels, measures, fit.oneLine]
    )
    return {
        layout: fitted.layout,
        isMeasured: rowWidth !== undefined,
        onRowWidth: setRowWidth,
        fit: { width, source: fitted.source },
    }
}
```

- [ ] **Step 5: The table writes the marker**

Create `core/types/react-native-dataset.d.ts`:

```ts
// react-native-web writes a View's dataSet as data-* attributes (keys hyphenated:
// fitWidth → data-fit-width); React Native's types do not declare it. Native
// ignores it. Used by the report table's e2e marker.
import 'react-native'

declare module 'react-native' {
    interface ViewProps {
        dataSet?: Record<string, string>
    }
}
```

In `core/package.json`, after `"./types/editor-webview": "./types/editor-webview.d.ts",` add `"./types/react-native-dataset": "./types/react-native-dataset.d.ts",`. In `core/tsconfig.package-base.json` `types`, after `"@tinycld/core/types/editor-webview"` add `"@tinycld/core/types/react-native-dataset"`.

In `core/reports/components/ReportTable.tsx`:
- `RowWidthProbe` (unchanged): `<View testID="report-width-probe" onLayout={e => onWidth(e.nativeEvent.layout.width)} />`.
- `ReportTableProps`, after `onRowWidth`:
  ```ts
      /** The width the layout was fitted to and the rule that laid it out (useTableLayout). */
      fit?: { width: number; source: LayoutSource }
  ```
  (import `type LayoutSource` from `'../lib/table-lines'`).
- In `ReportTable`, above the return:
  ```ts
      // The e2e marker (tests/e2e/helpers.ts waitForTableFit): set once the rows'
      // width is measured, so a test reads the layout of the width the rows have.
      const dataSet =
          props.isMeasured && !isStateShown && props.fit
              ? { fitWidth: String(props.fit.width), layout: props.fit.source }
              : undefined
  ```
  and `<View testID={testID} dataSet={dataSet} className={…}>`.

In `core/reports/components/ReportView.tsx` `TableBody`, after `onRowWidth={table.onRowWidth}` add `fit={table.fit}`. (`ReportBuilderPanel`'s sheet preview renders `ReportBody` → `TableBody` with its own `useTableLayout(preview)`, so it gets the fit with no change.)

- [ ] **Step 6: The e2e helper**

In `tests/e2e/helpers.ts`, add:

```ts
/**
 * Waits until the report table's layout is fitted to the width its rows have now
 * (the table's data-fit-width equals the row probe's width), and returns the rule
 * that laid it out: 'one-line', 'desktop', 'tablet', 'mobile' or 'auto'. Call it
 * after each setViewportSize before reading the table's geometry.
 */
export async function waitForTableFit(page: Page): Promise<string> {
    let layout = ''
    await expect
        .poll(
            async () => {
                layout = await page.evaluate(() => {
                    const table = document.querySelector<HTMLElement>('[data-testid="report-table"]')
                    const probe = table?.querySelector<HTMLElement>(
                        '[data-testid="report-width-probe"]'
                    )
                    if (!table || !probe || table.dataset.fitWidth === undefined) return ''
                    const drawn = probe.getBoundingClientRect().width
                    const isSettled = Math.abs(drawn - Number(table.dataset.fitWidth)) < 1
                    return isSettled ? (table.dataset.layout ?? '') : ''
                })
                return layout
            },
            { message: 'the report table fits its layout to the width its rows have' }
        )
        .not.toBe('')
    return layout
}
```

In `tests/e2e/reports-fit.spec.ts`, import `waitForTableFit` from `'./helpers'`; in the loop after `toHaveCSS('opacity', '1')` (`:127`) add:

```ts
                const layout = await waitForTableFit(p)
                // Eight columns never fit one line on a phone, and there is no arrangement.
                if (size.width === 390) expect(layout).toBe('auto')
```

- [ ] **Step 7: Typecheck, lint, e2e**

Run: `pnpm exec tinycld-pkg typecheck` then `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check core/reports/hooks/useTableLayout.ts core/reports/hooks/useReportView.ts core/reports/components/ReportTable.tsx core/reports/components/ReportView.tsx core/reports/lib/spec.ts core/reports/lib/__tests__/spec.test.ts core/types/react-native-dataset.d.ts tests/e2e/helpers.ts tests/e2e/reports-fit.spec.ts`
Expected: no errors. Then (ports free) `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-fit.spec.ts`
Expected: PASS.

- [ ] **Step 8: Stage**

`core/reports/lib/spec.ts core/reports/lib/__tests__/spec.test.ts core/reports/hooks/useReportView.ts core/reports/hooks/useTableLayout.ts core/reports/components/ReportTable.tsx core/reports/components/ReportView.tsx core/types/react-native-dataset.d.ts core/package.json core/tsconfig.package-base.json tests/e2e/helpers.ts tests/e2e/reports-fit.spec.ts`

---

### Task 12: Builder — column fit and the width gates

**Files:**
- Modify: `core/reports/lib/builder/draft.ts` (`fitIssues`, `NEVER_CUT`, `columnWithFit`, `layoutWithMinWidth`; `draftIssues` `:464-480`)
- Modify: `core/reports/lib/builder/errors.ts:17-49` (`display.columns` → Columns)
- Create: `core/reports/components/builder/ColumnFitField.tsx`
- Modify: `core/reports/components/builder/ColumnsSection.tsx:79-87`, `:89-152`, `:460-513` (pass `isFigure`)
- Modify: `core/reports/components/builder/ArrangeSection.tsx` (One line from; Use from; hint)
- Test: `core/reports/lib/builder/__tests__/draft.test.ts`, `core/reports/lib/builder/__tests__/errors.test.ts`
- Create: `tests/e2e/reports-column-fit.spec.ts`

**Interfaces:**
- Consumes: Task 8 `tidyFit`, `ColumnDraft.fit`, `BuilderDraft.display.oneLine`, `DisplayLayout.minWidth`; Task 9 `isFigureType`; Task 11 `waitForTableFit`; `columnTypes` (`formula-context.ts:173`), `useColumnChoices` (`report-formula.tsx:446`), `FieldError` (`parts.tsx:40`), `PickMenu`/`PickItem`; e2e `createInvitedUser`, `skipWithoutReportsStub` (`helpers.ts`), `openReportStub`, `addTeam`, `addSale`, `openReportsHub`, `startReport`, `addColumn`, `chooseOption`, `saveNewReport`, `uniqueName`, `SALES_SOURCE` (`reports-helpers.ts`).
- Produces: `NEVER_CUT: string`; `fitIssues(draft: BuilderDraft, catalog: Catalog): Issue[]` (path `['columns', i, 'fit']`); `columnWithFit(column: ColumnDraft, fit: ColumnFit | undefined): ColumnDraft`; `layoutWithMinWidth(layout: DisplayLayout, minWidth: string | undefined): DisplayLayout`; `ColumnFitField` (test ids `report-builder-column-<i>-fit-min`, `-fit-wrap`, `-fit-grow`, `-fit-never-cut`, `-fit-clear`, `-fit-problem`); Arrange test ids `report-builder-arrange-one-line`, `report-builder-arrange-use-from`.

- [ ] **Step 1: Write the failing unit tests**

Append to `core/reports/lib/builder/__tests__/draft.test.ts` (add `columnWithFit`, `layoutWithMinWidth`, `NEVER_CUT` to the `'../draft'` import):

```ts
describe('column fit choices', () => {
    const spec: ReportSpec = {
        version: CURRENT_SPEC_VERSION,
        source: SALES_KEY,
        columns: [
            { id: 'customer', label: 'Customer', path: [{ field: 'customer' }] },
            { id: 'amount', label: 'Amount', path: [{ field: 'amount' }] },
        ],
    }
    const draft = draftFromSpec(spec, META, CATALOG)
    const withFits = (customer: ColumnFit | undefined, amount: ColumnFit | undefined) => ({
        ...draft,
        columns: draft.columns.map(column =>
            columnWithFit(column, column.id === 'customer' ? customer : amount)
        ),
    })

    it('lets text narrow and wrap, and a figure take only extra room or its whole value', () => {
        const ok = withFits(
            { minWidth: '12ch', whiteSpace: 'normal', lineClamp: 2 },
            { minWidth: 'max-content', flexGrow: 2 }
        )
        expect(draftIssues(ok, CATALOG)).toEqual([])
        expect(builderSchema(CATALOG).safeParse(ok).success).toBe(true)
    })

    it('refuses a setting that would cut a figure, on the column', () => {
        const cut = withFits(undefined, { minWidth: '6ch' })
        expect(draftIssues(cut, CATALOG)).toEqual([{ path: ['columns', 1, 'fit'], message: NEVER_CUT }])
    })

    it('clears an empty fit', () => {
        const [customer] = draft.columns
        if (!customer) throw new Error('no column')
        expect(columnWithFit(customer, { flexGrow: undefined }).fit).toBeUndefined()
        expect(columnWithFit(customer, { flexGrow: 0, minWidth: '8ch' }).fit).toEqual({
            minWidth: '8ch',
            flexGrow: 0,
        })
    })

    it('puts a layout minWidth where the engine writes it', () => {
        const layout = startingLayout('tablet', [{ id: 'customer' }, { id: 'amount' }], 2)
        expect(Object.keys(layoutWithMinWidth(layout, '600px'))).toEqual([
            'width',
            'minWidth',
            'tracks',
            'cells',
        ])
        expect(layoutWithMinWidth({ ...layout, minWidth: '600px' }, undefined)).toEqual(layout)
    })
})
```

Append to `core/reports/lib/builder/__tests__/errors.test.ts` inside `describe('builder errors')`:

```ts
    it('shows a column fit problem under Columns, naming the column', () => {
        expect(sectionOf('display.columns.amount.minWidth')).toBe('columns')
        expect(sectionOf('display.oneLine.minWidth')).toBe('arrange')
        expect(
            describeProblem({
                path: 'display.columns.amount.minWidth',
                code: 'invalid',
                message: 'a figure is never cut',
            })
        ).toBe('Column amount: a figure is never cut')
        expect(
            describeProblem({ path: 'display.oneLine.minWidth', code: 'invalid', message: 'px only' })
        ).toBe('One line: px only')
    })
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/builder/__tests__/draft.test.ts core/reports/lib/builder/__tests__/errors.test.ts`
Expected: FAIL — `columnWithFit`, `layoutWithMinWidth`, `NEVER_CUT` not exported; `display.columns…` maps to `arrange`.

- [ ] **Step 3: Implement the draft rules**

In `core/reports/lib/builder/draft.ts` (import `isFigureType` from `'../column-measure'`; `columnTypes` is already imported):

```ts
export const NEVER_CUT =
    'Numbers, amounts, percents, dates and yes/no values are never cut: only Extra room can change'

// What a figure may take (core/server/reports/fit.go fitRefusals): flexGrow, and a
// minWidth that keeps the whole value.
const isWholeMin = (minWidth: string | undefined) =>
    minWidth === undefined || minWidth === 'auto' || minWidth === 'max-content'

function fitIssues(draft: BuilderDraft, catalog: Catalog): Issue[] {
    const types = columnTypes(draft, catalog)
    return draft.columns.flatMap((column, i): Issue[] => {
        const fit = tidyFit(column.fit)
        if (!fit || !isFigureType(types.get(column.id))) return []
        const { flexGrow: _flexGrow, minWidth, ...cutting } = fit
        if (isWholeMin(minWidth) && Object.keys(cutting).length === 0) return []
        return [{ path: ['columns', i, 'fit'], message: NEVER_CUT }]
    })
}

/** The column with this fit, tidied; an empty fit is removed. */
export function columnWithFit(column: ColumnDraft, fit: ColumnFit | undefined): ColumnDraft {
    return { ...column, fit: tidyFit(fit) }
}

/** The layout with this minWidth, its fields in the engine's order (width, minWidth, tracks, cells). */
export function layoutWithMinWidth(layout: DisplayLayout, minWidth: string | undefined): DisplayLayout {
    const { width, tracks, cells } = layout
    return minWidth ? { width, minWidth, tracks, cells } : { width, tracks, cells }
}
```

In `draftIssues` add `...fitIssues(draft, catalog),` after `...columnIssues(draft, catalog),`. (`columnWithFit` leaves `fit: undefined` on the object when empty; `tidyFit` and the zod `optional()` read it as absent.)

- [ ] **Step 4: Implement the error mapping**

In `core/reports/lib/builder/errors.ts`:
- `sectionOf`: replace `if (head === 'display') return 'arrange'` with
  ```ts
      // A column's fit is set on the column; the one-line width and layouts under Arrange.
      if (head === 'display') return specPath.startsWith('display.columns.') ? 'columns' : 'arrange'
  ```
- `describeProblem`, at its start:
  ```ts
      const parts = problem.path.split('.')
      // display.columns.<col>.<setting> names its column by id; display.oneLine is the one-line width.
      if (parts[0] === 'display' && parts[1] === 'columns' && parts[2])
          return `Column ${parts[2]}: ${problem.message}`
      if (parts[0] === 'display' && parts[1] === 'oneLine') return `One line: ${problem.message}`
  ```
  (the existing `const parts` line moves up to here.)

Run the unit tests again. Expected: PASS.

- [ ] **Step 5: The column picker**

Create `core/reports/components/builder/ColumnFitField.tsx`:

```tsx
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { X } from 'lucide-react-native'
import { Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { type ColumnDraft, columnWithFit } from '../../lib/builder/draft'
import type { ColumnFit } from '../../lib/spec'
import { FieldError, IconButton } from './parts'

// Design §3.6: a text column sets its narrowest width, whether long text wraps and
// its share of extra room; a figure is never cut, so it sets only extra room. Width,
// widest and % settings are kept as read (CLI or API) and not offered here.

const NARROWEST: { id: string; label: string; minWidth?: string }[] = [
    { id: 'auto', label: 'Automatic' },
    { id: 'whole', label: 'Whole value', minWidth: 'max-content' },
    { id: '6', label: '6 letters', minWidth: '6ch' },
    { id: '8', label: '8 letters', minWidth: '8ch' },
    { id: '12', label: '12 letters', minWidth: '12ch' },
    { id: '16', label: '16 letters', minWidth: '16ch' },
    { id: '24', label: '24 letters', minWidth: '24ch' },
]
const LONG_TEXT: { id: string; label: string; lines?: number }[] = [
    { id: 'one', label: 'One line, ends in …' },
    { id: '2', label: 'Wrap to 2 lines', lines: 2 },
    { id: '3', label: 'Wrap to 3 lines', lines: 3 },
]
// "Normal" is a text column's default; a figure's default is "None".
const EXTRA_ROOM = [
    { id: '0', label: 'None', grow: 0 },
    { id: '1', label: 'Normal', grow: 1 },
    { id: '2', label: 'Double', grow: 2 },
    { id: '3', label: 'Triple', grow: 3 },
]

function useColumnFit(
    builder: ReportBuilder,
    columns: ColumnDraft[],
    index: number,
    isFigure: boolean
) {
    const column = columns[index]
    const fit = column?.fit ?? {}
    const setFit = (next: ColumnFit) =>
        builder.setColumns(columns.map((c, i) => (i === index ? columnWithFit(c, next) : c)))
    const lines = fit.whiteSpace === 'normal' ? (fit.lineClamp ?? 2) : undefined
    // The measure's default share (column-measure.ts): text 1, a figure 0.
    const defaultGrow = isFigure ? 0 : 1
    const grow = fit.flexGrow ?? defaultGrow
    const narrowest: PickItem[] = NARROWEST.map(item => ({
        id: item.id,
        label: item.label,
        isSelected: item.minWidth === fit.minWidth,
        onSelect: () => setFit({ ...fit, minWidth: item.minWidth }),
    }))
    const longText: PickItem[] = LONG_TEXT.map(item => ({
        id: item.id,
        label: item.label,
        isSelected: item.lines === lines,
        onSelect: () =>
            setFit({ ...fit, whiteSpace: item.lines ? 'normal' : undefined, lineClamp: item.lines }),
    }))
    // The column's default share is not written.
    const extraRoom: PickItem[] = EXTRA_ROOM.map(item => ({
        id: item.id,
        label: item.label,
        isSelected: item.grow === grow,
        onSelect: () =>
            setFit({ ...fit, flexGrow: item.grow === defaultGrow ? undefined : item.grow }),
    }))
    const error = builder.form.getFieldState(`columns.${index}.fit`, builder.form.formState).error
    return {
        column,
        narrowest,
        longText,
        extraRoom,
        narrowestLabel: NARROWEST.find(item => item.minWidth === fit.minWidth)?.label ?? fit.minWidth ?? '',
        longTextLabel: LONG_TEXT.find(item => item.lines === lines)?.label ?? '',
        extraRoomLabel: EXTRA_ROOM.find(item => item.grow === grow)?.label ?? String(grow),
        hasFit: column?.fit !== undefined,
        error: error?.message,
        onClear: () => setFit({}),
    }
}

interface ColumnFitFieldProps {
    builder: ReportBuilder
    columns: ColumnDraft[]
    index: number
    /** The column shows numbers, amounts, percents, dates or yes/no values: never cut. */
    isFigure: boolean
}

export function ColumnFitField({ builder, columns, index, isFigure }: ColumnFitFieldProps) {
    const fit = useColumnFit(builder, columns, index, isFigure)
    if (!fit.column) return null
    const testID = `report-builder-column-${index}-fit`
    const name = fit.column.label
    return (
        <View className="gap-1">
            <View className="flex-row flex-wrap items-center gap-2">
                <TextFitMenus isVisible={!isFigure} fit={fit} name={name} testID={testID} />
                <NeverCut isVisible={isFigure} testID={testID} />
                <PickMenu
                    label={`Extra room: ${fit.extraRoomLabel}`}
                    purpose={`${name} extra room`}
                    items={fit.extraRoom}
                    testID={`${testID}-grow`}
                />
                <IconButton
                    label={`Clear ${name} fit`}
                    testID={`${testID}-clear`}
                    isVisible={fit.hasFit && fit.error !== undefined}
                    icon={X}
                    onPress={fit.onClear}
                />
            </View>
            <FieldError message={fit.error} testID={`${testID}-problem`} />
        </View>
    )
}

type ColumnFitState = ReturnType<typeof useColumnFit>

function TextFitMenus({
    isVisible,
    fit,
    name,
    testID,
}: {
    isVisible: boolean
    fit: ColumnFitState
    name: string
    testID: string
}) {
    if (!isVisible) return null
    return (
        <>
            <PickMenu
                label={`Narrowest: ${fit.narrowestLabel}`}
                purpose={`${name} narrowest width`}
                items={fit.narrowest}
                testID={`${testID}-min`}
            />
            <PickMenu
                label={`Long text: ${fit.longTextLabel}`}
                purpose={`${name} long text`}
                items={fit.longText}
                testID={`${testID}-wrap`}
            />
        </>
    )
}

function NeverCut({ isVisible, testID }: { isVisible: boolean; testID: string }) {
    if (!isVisible) return null
    return (
        <Text testID={`${testID}-never-cut`} className="text-xs text-muted-foreground">
            Never cut
        </Text>
    )
}
```

`IconButton` (`parts.tsx:22`) takes `label`, `testID`, `isVisible`, `icon`, `onPress`; `X` from `lucide-react-native` is already used in `ReportBuilderPanel.tsx:18`.

In `core/reports/components/builder/ColumnsSection.tsx`:
- import `isFigureType` from `'../../lib/column-measure'` and `ColumnFitField` from `'./ColumnFitField'`;
- `ColumnRowProps` gains `/** The columns that are never cut (numbers, amounts, percents, dates, yes/no). */ figureIds: ReadonlySet<string>`;
- in `ColumnRow`, after `<FormatField … />`: `<ColumnFitField builder={builder} columns={columns} index={index} isFigure={figureIds.has(column.id)} />`;
- in `ColumnsSection`, after `numericIds`: `const figureIds = new Set(choices.filter(choice => isFigureType(choice.type)).map(choice => choice.key))`, and pass `figureIds={figureIds}` to `<ColumnRow>` (`:506-513`).

- [ ] **Step 6: Arrange — One line from, Use from**

In `core/reports/components/builder/ArrangeSection.tsx` (import `layoutWithMinWidth` from `'../../lib/builder/draft'`):

```ts
// The least row width for one line (display.oneLine), and for an arrangement
// (layouts[].minWidth): below it the report uses another layout.
const GATES: { id: string; label: string; minWidth?: string }[] = [
    { id: 'any', label: 'Any width' },
    { id: '600', label: '600 px', minWidth: '600px' },
    { id: '800', label: '800 px', minWidth: '800px' },
    { id: '1040', label: '1040 px', minWidth: '1040px' },
    { id: '1280', label: '1280 px', minWidth: '1280px' },
]

const gateItems = (current: string | undefined, onSelect: (minWidth?: string) => void): PickItem[] =>
    GATES.map(gate => ({
        id: gate.id,
        label: gate.label,
        isSelected: gate.minWidth === current,
        onSelect: () => onSelect(gate.minWidth),
    }))

const gateLabel = (current: string | undefined) =>
    GATES.find(gate => gate.minWidth === current)?.label ?? current ?? 'Any width'
```

In `useArrange` add to the returned object:

```ts
        oneLineItems: gateItems(display.oneLine?.minWidth, minWidth =>
            builder.setDisplay(
                minWidth ? { ...display, oneLine: { minWidth } } : { layouts: display.layouts }
            )
        ),
        oneLineLabel: gateLabel(display.oneLine?.minWidth),
        useFromItems: gateItems(layout?.minWidth, minWidth => {
            if (layout) setLayout(layoutWithMinWidth(layout, minWidth))
        }),
        useFromLabel: gateLabel(layout?.minWidth),
```

and change `hint` to: `'Used where the rows are at least the Use from width, the columns do not fit on one line, and this arrangement fits. Each line gives its columns the room their content needs.'`

Add the components:

```tsx
function OneLineFrom({ arrange }: { arrange: Arrange }) {
    return (
        <View className="flex-row items-center gap-2">
            <Text className="flex-1 text-sm text-foreground">One line from</Text>
            <PickMenu
                label={arrange.oneLineLabel}
                purpose="Least row width for one line"
                items={arrange.oneLineItems}
                testID="report-builder-arrange-one-line"
            />
        </View>
    )
}
```

Render `<OneLineFrom arrange={arrange} />` in `ArrangeSection` above the tablist. In `ArrangeFields`, after the "Columns per line" row, add the same row shape labelled "Use from" with `label={arrange.useFromLabel}`, `purpose="Least row width for this arrangement"`, `items={arrange.useFromItems}`, `testID="report-builder-arrange-use-from"`. Change `AutoCaption`'s text to start "Off: the columns fit on one line when the rows are wide enough and the columns fit." and keep the rest.

- [ ] **Step 7: Write the e2e test**

Create `tests/e2e/reports-column-fit.spec.ts`:

```ts
import { expect, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub, waitForTableFit } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    chooseOption,
    openReportStub,
    openReportsHub,
    SALES_SOURCE,
    saveNewReport,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * Spec v4 column fit set in the builder: a figure is never cut, the report is one
 * line only from its one-line width, an arrangement is used only from its Use from
 * width, and a phone below both gets automatic lines.
 */
test.describe('report column fit', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('one line from 1040, a tablet arrangement from 600, automatic lines on a phone', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptcfit')
        try {
            const team = uniqueName('Fit Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'Fitco 01',
                amount: 12345,
                cost: 100,
                day: '2026-01-05',
                status: 'paid',
            })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            // An amount is never cut: it offers only Extra room.
            await expect(p.getByTestId('report-builder-column-1-fit-never-cut')).toBeVisible()
            await expect(p.getByTestId('report-builder-column-1-fit-min')).toHaveCount(0)
            await chooseOption(p, 'report-builder-column-0-fit-min', '12')
            await expect(p.getByTestId('report-builder-column-0-fit-min')).toContainText('12 letters')
            await p.getByTestId('report-builder-section-arrange').click()
            await chooseOption(p, 'report-builder-arrange-one-line', '1040')
            await p.getByTestId('report-builder-arrange-width-tablet').click()
            await p.getByTestId('report-builder-arrange-on').click()
            await chooseOption(p, 'report-builder-arrange-use-from', '600')
            // Turning an arrangement on keeps the one-line width.
            await expect(p.getByTestId('report-builder-arrange-one-line')).toContainText('1040 px')
            await saveNewReport(p, uniqueName('Column fit'))

            // Rows on /a/reports are the window less the 64 px rail: 1216 and 960.
            await p.setViewportSize({ width: 1280, height: 800 })
            expect(await waitForTableFit(p)).toBe('one-line')
            await p.setViewportSize({ width: 1024, height: 800 })
            expect(await waitForTableFit(p)).toBe('tablet')
            // A phone has no rail: 390 is under both gates.
            await p.setViewportSize({ width: 390, height: 844 })
            expect(await waitForTableFit(p)).toBe('auto')
        } finally {
            await close()
        }
    })
})
```

- [ ] **Step 8: Run unit tests, typecheck, lint, e2e**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/builder/__tests__/draft.test.ts core/reports/lib/builder/__tests__/errors.test.ts`, then `pnpm exec tinycld-pkg typecheck`, then `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check core/reports/lib/builder/draft.ts core/reports/lib/builder/errors.ts core/reports/components/builder/ColumnFitField.tsx core/reports/components/builder/ColumnsSection.tsx core/reports/components/builder/ArrangeSection.tsx tests/e2e/reports-column-fit.spec.ts`, then (ports free) `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-column-fit.spec.ts`
Expected: all PASS.

- [ ] **Step 9: Stage**

`core/reports/lib/builder/draft.ts core/reports/lib/builder/errors.ts core/reports/lib/builder/__tests__/draft.test.ts core/reports/lib/builder/__tests__/errors.test.ts core/reports/components/builder/ColumnFitField.tsx core/reports/components/builder/ColumnsSection.tsx core/reports/components/builder/ArrangeSection.tsx tests/e2e/reports-column-fit.spec.ts`

---

### Task 13: Wrap inside cells (web and native); help

**Files:**
- Modify: `core/reports/components/ReportRow.tsx:208-217` (`LabelText`), `:219-227` (`RowLabelProps`), `:245-288` (`GroupToggleLabel`, `RowLabel`), `:309-366` (`RowCell`), `:392-429` (`OneLineRow`), `:431-475` (`BlocksRow`)
- Modify: `tests/e2e/reports-column-fit.spec.ts` (second test)
- Modify: `core/help/reports.md:119-127`

**Interfaces:**
- Consumes: Task 10 `LineLayout.lines`; Task 12 `report-builder-column-<i>-fit-wrap`; Task 11 `waitForTableFit`; e2e `addSales` (`reports-helpers.ts:108`).
- Produces: `RowLabelProps.lines?: number`; `RowCellProps.lines?: number`. No new exports.

There is no front-end unit test (CLAUDE.md: UI behaviour is covered by e2e). `numberOfLines` is React Native's own prop: on web react-native-web draws it with `-webkit-line-clamp`, on native the Text wraps to that many lines and ends in "…"; FlashList measures each row, so a two-line row gets its height on both.

- [ ] **Step 1: Write the failing e2e test**

Add to the `describe` in `tests/e2e/reports-column-fit.spec.ts` (import `addSales` too):

```ts
    test('long text set to wrap takes two lines on a phone and one on a wide screen', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptwrap2')
        try {
            const team = uniqueName('Wrap Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSales(p, {
                team,
                customerPrefix: 'Fitco Wholesale Partners of the Greater Northwest Region Cooperative',
                count: 1,
                firstDay: '2026-01-05',
                firstAmount: 12345,
                cost: 100,
                quantity: 1,
            })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-0-fit-wrap', '2')
            await saveNewReport(p, uniqueName('Wrap'))
            // The customer is the label of each detail row; its text is the row's first text.
            const label = p.locator('[data-testid^="report-row-d:"]').first().locator('div[dir="auto"]')
            const height = async () => (await label.boundingBox())?.height ?? 0

            await p.setViewportSize({ width: 1280, height: 800 })
            await waitForTableFit(p)
            // text-sm is 20 px a line.
            await expect.poll(height).toBeLessThan(26)
            // Two columns still fit one line on a phone (Σmin ≈ 230 < 375), so the label
            // wraps inside its one-line box (about 260 px of room for about 500 px of text).
            await p.setViewportSize({ width: 390, height: 844 })
            expect(await waitForTableFit(p)).toBe('one-line')
            await expect.poll(height).toBeGreaterThan(30)
            await expect.poll(height).toBeLessThan(46)
        } finally {
            await close()
        }
    })
```

Run (ports free): `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-column-fit.spec.ts`
Expected: the new test FAILS at 390 (the label stays one line, about 20 px, cut with "…": `numberOfLines={1}`); the first test passes. The phone run is not a duplicate: only at 390 is the label box narrower than its text.

- [ ] **Step 2: Implement**

In `core/reports/components/ReportRow.tsx`:

1. `LabelText`:
   ```tsx
   function LabelText({ row, text, lines = 1 }: { row: ResultRow; text: string; lines?: number }) {
       return (
           <Text
               numberOfLines={lines}
               className={`min-w-0 flex-1 text-sm ${labelTone(row.kind)} ${ROW_WEIGHT[row.kind]}`}
           >
               {text}
           </Text>
       )
   }
   ```
2. `RowLabelProps`: add `/** The lines its text may wrap to (the layout's lines); absent: one. */ lines?: number`. `GroupToggleLabel` and `RowLabel` read `lines` from their props and pass `<LabelText row={…} text={text} lines={lines} />`.
3. `RowCellProps`: add the same `lines?: number`; `RowCell` destructures `lines = 1` and sets `numberOfLines={lines}` on its `Text`. Figure columns never get `lines` (the measure gives no `lines` to a figure), so figures stay on one line.
4. `OneLineRow`: pass `lines={label ? layout.lines?.[label.id] : undefined}` to `RowLabel`, and `lines={layout.lines?.[column.id]}` to each `RowCell`. Update the comment above it: "…a cell too narrow for its text ends in "…" (numberOfLines), or wraps to the lines its fit allows, so nothing runs past the table's edge."
5. `BlocksRow`: rename the local `lines` (the row's layout) to `rowLines` (`const rowLines = rowLayout(…)`, `rowLines.kind`, `rowLines.root`, `rowLines.indent`), and in `renderCell` pass `lines={rowLines.lines?.[col]}` to both `RowLabel` and `RowCell`.

Run the e2e again. Expected: PASS (both tests).

- [ ] **Step 3: Help**

In `core/help/reports.md`, "When columns do not fit" (`:119-127`): in the paragraph at `:125`, change "When the columns do not fit on one line, the report uses the first arrangement that fits the screen" to "When the columns do not fit on one line (or the rows are narrower than **One line from**), the report uses the first arrangement that fits the screen and that the rows reach (its **Use from**)". After that paragraph that starts "When the columns do not fit on one line, the report uses the first arrangement…", add:

```markdown
To change how a column fits, open the column in **Columns**:

- **Narrowest** is the least width a text column gets before the report wraps its rows: **Automatic** (about eight letters), **Whole value** (never cut), or a number of letters.
- **Long text** keeps the text on **One line, ends in …**, or wraps it to 2 or 3 lines.
- **Extra room** is the column's share of the spare room on one line: **None**, **Normal**, **Double** or **Triple**.

Numbers, amounts, percents, dates and yes/no values show **Never cut**: you can change only their extra room.

In **Arrange**, **One line from** is the least row width at which the report uses one line; below it, the report uses an arrangement or wraps its rows. **Use from** on an arrangement is the least row width at which it is used. On a computer the rows are the window's width less the side rail.
```

Then run `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && pnpm run packages:generate` (the help index is generated).

- [ ] **Step 4: Typecheck and lint**

Run: `pnpm exec tinycld-pkg typecheck` then `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check core/reports/components/ReportRow.tsx tests/e2e/reports-column-fit.spec.ts`
Expected: no errors.

- [ ] **Step 5: Stage**

`core/reports/components/ReportRow.tsx tests/e2e/reports-column-fit.spec.ts core/help/reports.md` (the generator's output is gitignored).

---

## Books follow-up (not core tasks; after the core merge, in the Books lane)

Part 1's section stands (GL and TB `Display` settings, the Books Go tests the version change breaks, the GL and TB e2e changes). Part 2 adds or confirms:

- `waitForTableFit(page)` is exported by core `tests/e2e/helpers.ts` (Task 11). Books e2e imports it the way it imports the other core helpers; it returns `'one-line' | 'desktop' | 'tablet' | 'mobile' | 'auto'`. It reads the row probe `[data-testid="report-width-probe"]`; a Books spec that reads the probe itself uses that id.
- GL (`general-ledger.spec.ts:921-966`): with `OneLine.MinWidth: "1040px"` and the tablet layout's `MinWidth: "600px"`, expect `'tablet'` at 900 and 1024 (rows 836 and 960) and `'one-line'` at 1280 (1216). GL phone (`:861-919`): expect `'mobile'` (390 has no gate on the mobile layout).
- TB (`trial-balance.spec.ts:553-613`): `account` with `WhiteSpace: "normal", LineClamp: 2` wraps through Task 13 (`numberOfLines` from `layout.lines`); the label checks read `label.top`/`label.bottom`, which hold for a two-line label.
- `books/tinycld/books/screens/helix-mockup.tsx` builds a `ReportViewModel` with no `fit`; Task 11 makes `fit` optional, so it compiles and draws with the default fit. No change needed.
- The GL `type` column's `MinWidth: "12ch"` is drawn as `ceil(12 × CH_WIDTH) + 24` px (Task 9: ch lengths are text widths plus padding).

## Open questions and risks

**Answered 2026-11-08 by the human:** plan approved; `flexGrow` applies on one line only (not on automatic lines); the "For review" choices stand.

Open questions (not covered by the decisions): one — `flexGrow` on automatic lines (design §3.2) is not applied; see "For review". The plan runs without the answer; the follow-up is one change in `fitBlock`. The other choices in "For review" (units, wrap floor, flexGrow on one line only, empty fit dropped) are this plan's and can be changed in review without touching another task's interface.

Risks:
- `waitForTableFit` compares the probe's `getBoundingClientRect().width` with the `onLayout` width (tolerance 1 px). If react-native-web reports `onLayout` widths rounded differently under a zoom factor, the poll would not settle; the e2e runs at zoom 1.
- `dataSet` on native: React Native ignores the unknown prop on `View`. If a future RN version warns on it, gate it with `Platform.OS === 'web'`.
- A wrapped label on a group row that is stuck at the top (`report-sticky-row`) draws the same `numberOfLines`; the sticky copy's height follows its content. Not covered by an e2e (no group in the wrap test); the GL/TB Books e2e have groups and exercise it once Books sets `lineClamp`.
- Phone GL line 4 (Part 1 risk) is still decided by the Books follow-up's settings; Task 10's max cap and `flexGrow` give it the means.
