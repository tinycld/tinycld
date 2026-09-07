# Overlay engine — build order and package migration

**Status:** Core engine and core screens done (2026-09-06); package migrations in progress. Reference: `docs/overlays.md`.

**Decisions (settled with the maintainer, 2026-09-06):**

- Menus and dialogs on the mobile breakpoint render as bottom sheets by
  default; `presentation="popover"` / `presentation="dialog"` pin the desktop
  form.
- Menu items are prop-based (`label`, `icon`, `onSelect`, …), not
  children-based. A sheet renders the same props as touch rows.
- The menubar and the search palette move onto the engine in this pass.
- Dialog widths: sm 360, md 420, lg 480, xl 640, full. Nothing else.
- Every package migrates immediately after the engine lands; the old
  compound APIs are deleted, not deprecated.

## Build order (core) — done

1. **Overlay host** — `core/ui/overlay/`: layer host (RN Modal on native,
   portal on web), dismissal (`shouldDismiss()` pure predicate + the document
   listener), Escape via the shortcut scope, focus trap and restore,
   predefined animations. Replaces gluestack's overlay for our surfaces; the
   provider stays mounted until the last gluestack consumer is gone.
2. **Popover** — `core/ui/popover/`: `placePopover()` pure function with tests,
   trigger measurement (pointer, keyboard, controlled, point anchor), height
   cap with internal scroll, the submenu slot, `hasTextInput`. Sheet
   presentation on mobile.
3. **Sheet** — `core/ui/sheet/`: `BottomDrawer` renamed and given title,
   body and footer. `BottomDrawer` becomes a re-export until its nine direct
   consumers move.
4. **Dialog** — sizes `xl` and `full`, `header` slot, `presentation`. Raw
   `Modal` exports removed from `core/ui/modal` (the file stays as the
   internal primitive Dialog is built on until the host in step 1 replaces
   gluestack's Modal creator entirely).
5. **Menu** — rewritten on Popover with the prop-based item API, roving
   arrow-key focus and typeahead, `Menu.Sub`, `Menu.Section`,
   `Menu.CheckboxItem`, `Menu.Custom`, sheet presentation.
6. **ContextMenu**, **menubar**, **search palette** onto the engine.
   `components/DropdownMenu.tsx` (`ToolbarMenu`, `DotsMenu`, `MenuActionItem`,
   `MenuCheckboxItem`, `MenuSectionLabel`) folds into `Menu` and is deleted.

## Package migration

Counts are files, from `rg` on 2026-09-06.

| Package | Menu | Dialog | BottomDrawer | ContextMenu / triggerPosition | Menubar |
|---|---|---|---|---|---|
| boards | 22 | 16 (done, on `Dialog`) | 1 | 0 | 0 |
| calc | 14 | 4 | 0 | 5 | 8 |
| calendar | 5 | 3 | 1 | 0 | 0 |
| contacts | 2 | 0 | 0 | 0 | 0 |
| drive | 6 | 5 | 0 | 1 | 0 |
| mail | 4 | 0 | 0 | 0 | 0 |
| text | 6 | 0 | 2 | 0 | 7 |
| google-takeout-import | 0 | 0 | 0 | 0 | 0 |
| tinycld (core screens) | 12 | 9 | 5 | 2 | 0 |

Order: core screens first (they prove every surface), then boards (the
largest menu consumer and already on `Dialog`), then calc and text (menubar
and the calc grid's point-anchored menus), then drive, calendar, mail,
contacts.

Each package migration is one PR that:

1. Replaces every `Menu`/`Modal`/`BottomDrawer` import with the new surfaces.
2. Deletes any local header, footer, close button, fixed-height ScrollView or
   outside-click hook the surface now owns.
3. Runs the package's unit and e2e suites. The e2e specs that pin each
   surface are the acceptance test; none may be weakened.

## Acceptance

- `pnpm run pkg:check` green across the workspace.
- Every member's e2e suite green.
- `rg "ui/modal'|Menu.Portal|Menu.Overlay|Menu.Content|BottomDrawer|MenuActionItem"`
  over the workspace returns only `core/ui/**`.
- Playwright measurements from this session's repro (submenu row alignment,
  dialog growth after its animation, settings dialog capped at 90% with a
  visible footer) still hold, on every migrated package.
