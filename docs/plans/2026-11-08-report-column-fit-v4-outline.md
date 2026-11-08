# Report column fit (spec v4) — outline

Spec: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/column-fit-design.md` with the binding decisions `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/column-fit-decisions.md` (incl. the 2026-11-08 "no backward compatibility" bullet). Background: `cut-1024-diagnosis.md`, `tanstack-table-assessment.md`, `tb-phone-diagnosis.md` (same folder).

Parts: `2026-11-08-report-column-fit-v4-part-1.md` (Tasks 1–7, the engine and the one spec version), `2026-11-08-report-column-fit-v4-part-2.md` (Tasks 8–14, the client, builder, e2e, help). Paths are relative to `/Users/nas/tmp/tinycld/books-purchases/tinycld`.

## No backward compatibility (human, 2026-11-08)

One spec version, 4. No upgrade path, no old-format readers, no "v3 reports draw the same" guarantee. Every fixture moves to `"version":4`. The upgrade steps, `LoadInfo.UpgradedFrom`, `Notice.FromVersion`, the `upgraded` notice and the client's `fromVersion` are deleted (Task 7). The fit defaults (`auto`) stay today's rules because no cleaner default was found that the author settings do not cover better (Task 9 says so, and changes one default: a figure-typed label column is never cut).

## Files

| File | Change | Task |
|---|---|---|
| `core/server/reports/spec.go` | `Display.OneLine`, `Display.Columns`, `DisplayLayout.MinWidth`, `ColumnFit`, `DisplayOneLine`; `CurrentVersion = 4` (T1); upgrade machinery deleted, `Load` reads only version 4 (T7) | 1, 7 |
| `core/server/reports/fit.go` (new) | length grammar, `fitLengthProblem`, `(b *binder) columnFits`, `fitRefusals` (figures never cut) | 1, 2 |
| `core/server/reports/display.go` | calls `columnFits`; layout `minWidth`; placement + pinwheel removed, per-cell range checks kept | 1, 3 |
| `core/server/reports/assemble.go`, `run.go` | `(c *compiler) resultType` extracted; `compileDry` calls `fitRefusals` (T2); `upgraded` notice + `Notice.FromVersion` removed (T7) | 2, 7 |
| `core/server/reports/v4_test.go` (new) | v4 decode, store, refusals | 1, 2 |
| `core/server/reports/*_test.go` (about 60 files) | `"version":1/2/3` → `"version":4` | 4, 5, 6 |
| `core/server/reports/v2_test.go`, `v3_test.go`, `spec_test.go`, `run_test.go`, `balance_shape_test.go`, `display_cases_test.go` | version/upgrade/placement tests rewritten or deleted | 1, 3, 7 |
| `core/reports/lib/spec.ts` | `CURRENT_SPEC_VERSION = 4` (T1); `version: z.literal(4)` (T7); `columnFitSchema`, `displayOneLineSchema`, layout `minWidth`, `FIT_LENGTH`, `parseFitLength` (T8) | 1, 7, 8 |
| `core/reports/lib/types.ts`, `components/ReportNotices.tsx` | `fromVersion` removed | 7 |
| `core/reports/lib/table-lines.ts` | `ARRANGEMENT_NOTICE` (T3); gates, `maxWidth` cap, `%`, `fitTable`, `lines` (T10) | 3, 10 |
| `core/reports/lib/column-measure.ts` | `measureColumns({ …, fits })` resolves `ColumnFit` | 9 |
| `core/reports/lib/builder/draft.ts` | keeps `display.oneLine`, `display.columns`, layout `minWidth` (T8); fit fields + `fitIssues` (T13) | 8, 13 |
| `core/reports/hooks/useTableLayout.ts`, `useReportView.ts`, `components/ReportView.tsx`, `components/builder/ReportBuilderPanel.tsx` | fit settings to the layout; `TableLayout.fit` | 11 |
| `core/reports/components/ReportTable.tsx`, `core/types/react-native-dataset.d.ts` (new) | `data-fit-width` / `data-layout`, probe `testID` | 11 |
| `core/reports/components/ReportRow.tsx` | `numberOfLines` from `layout.lines` | 12 |
| `core/reports/components/builder/ColumnsSection.tsx`, `ColumnFitField.tsx` (new), `ArrangeSection.tsx` | per-column width/min/max/wrap; one-line min width | 13 |
| `tests/e2e/helpers.ts` | `waitForTableFit(page): Promise<string>` | 11 |
| `tests/e2e/reports-fit.spec.ts`, `reports-column-fit.spec.ts` (new), `reports-shipped.spec.ts`, `reports-share.spec.ts` | marker use; v4 behaviour; version literals | 7, 11, 14 |
| `tests/scripts/reports-stub/server/register.go.txt` | shipped stub specs at version 4 | 7 |
| `core/help/reports.md`, `docs/packages.md` | "When columns do not fit"; display rows; "Spec version 4" | 14 |

## Tasks

1. **Go spec v4 format and definition checks.** Produces `Display{OneLine *DisplayOneLine; Columns map[string]ColumnFit; Layouts}`, `DisplayOneLine{MinWidth string}`, `DisplayLayout.MinWidth string`, `ColumnFit{Width, MinWidth, MaxWidth, WhiteSpace string; LineClamp int; TextOverflow string; FlexGrow *int}`, `CurrentVersion = 4` (identity step 3 kept until Task 7), `fitLengthProblem(s string, rule lengthRule) string`, `(b *binder) columnFits(p *plan)`, `v4_test.go` (`everyV4`); TS `CURRENT_SPEC_VERSION = 4`.
2. **Go: a figure is never cut.** Produces `(c *compiler) resultType(cp *colPlan) (typ, total FieldType)`, `(c *compiler) neverCut(cp *colPlan, first bool) bool`, `fitRefusals(c *compiler) error` called from `compileDry`; `neverCutMessage`.
3. **Placement leaves Go.** Go keeps per-cell range checks (`cellRangeProblem(tracks int, c DisplayCell) string`), drops `placeCells`/`cutDown`/`uncuttable`/`pinwheelMessage`/`prefixProblem`; TS `ARRANGEMENT_NOTICE` when an authored layout cannot be placed.
4. **Fixtures to v4, files a–h** (Go test literals).
5. **Fixtures to v4, files j–p.**
6. **Fixtures to v4, files r–v.**
7. **One spec version.** `Load` refuses any version but 4 (`version must be 4`); upgrade machinery, `LoadInfo.UpgradedFrom`, `Notice.FromVersion`, `upgraded` notice, TS `fromVersion` deleted; TS `version: z.literal(CURRENT_SPEC_VERSION)`; remaining v2/v3/spec/run tests, TS tests, e2e specs and the stub at version 4.
8. **TS spec v4 schema and builder round trip.** Produces `FIT_LENGTH: RegExp`, `type FitLength = { value: number; unit: 'ch' | 'px' | '%' }`, `parseFitLength(s: string): FitLength | null`, `columnFitSchema`, `type ColumnFit`, `displayOneLineSchema`, `type DisplayOneLine`, `displayLayoutSchema` + `minWidth?: string`, `reportSpecSchema.display` = `{ oneLine?, columns?: Record<string, ColumnFit>, layouts? }`, `type DisplayFit = { oneLine?: DisplayOneLine; columns?: Record<string, ColumnFit> }`; `BuilderDraft.display` = `{ oneLine?: DisplayOneLine; layouts }`, `ColumnDraft.fit?: ColumnFit`; `draftFromSpec` / `specFromDraft` keep them.
9. **Measure resolves column fit.** `ColumnMeasure = { natural; min; grow; max?: number; lines?: number; pct?: { width?: number; min?: number; max?: number } }`; `measureColumns({ columns, rows, format, levels, fits?: Record<string, ColumnFit> })`; `measureKey({ …, fits? })`; `CH_WIDTH = textWidth('0', CELL)`; default change: a figure-typed label column's min is its natural.
10. **Fit reads the settings.** `atWidth(m: ColumnMeasure, lineWidth: number): ColumnMeasure`; `shareWidth` caps grow at `max`; `type LayoutSource = 'one-line' | DisplayWidth | 'auto'`; `fitTable(input & { oneLine?: DisplayOneLine }): { layout: LineLayout; source: LayoutSource }`; `tableLines` = `fitTable(…).layout`; `LineLayout` gains optional `lines?: Record<string, number>`; layout `minWidth` gate.
11. **Fit settings reach the view; layout-settled marker.** `ReportViewModel.fit?: DisplayFit`; `TableLayout.fit: { width: number; source: LayoutSource }`; `ReportTableProps.fit?`; `report-table` `dataSet={{ fitWidth, layout }}` when measured; `RowWidthProbe` `testID="report-width-probe"` (kept); `core/types/react-native-dataset.d.ts`; `waitForTableFit(page): Promise<string>` in `tests/e2e/helpers.ts`; `reports-fit.spec.ts` waits with it.
12. **Wrap inside cells (web and native).** `LabelText`, `RowCell` and the one-line heading read `layout.lines?.[col] ?? 1` → `numberOfLines`.
13. **Builder: column fit and one-line width.** `ColumnFitField` (Width / Narrowest / Widest / Long text pickers on text columns; "Never cut" on figures), `ArrangeSection` "One line from"; `fitIssues(draft, catalog): Issue[]` in `draftIssues`; `setDisplay` keeps `oneLine`.
14. **E2E for v4, help, docs.** `tests/e2e/reports-column-fit.spec.ts` (desktop + phone 390); `core/help/reports.md` "When columns do not fit"; `docs/packages.md` display rows and "Spec version 4".

## Books follow-up (not core tasks)

In part 1's last section (written there because Task 1 and Task 7 already break Books Go tests) and repeated, with any Part 2 additions, in part 2's last section: GL and TB settings, the Books Go tests the version change breaks, and the Books e2e changes.
