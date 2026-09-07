# Overlays: Dialog, Sheet, Popover, Menu

Everything that floats above the screen — a dialog, a bottom sheet, a dropdown
menu, a context menu, an anchored picker, the search palette — is one of three
surfaces on one engine. A package never touches the engine; it writes the
surface's content and the engine owns everything the content used to get wrong.

| Surface | Import | Use it for |
|---|---|---|
| `Dialog` | `@tinycld/core/ui/dialog` | Forms, confirms, prompts, managers, pickers with a footer, lightboxes |
| `Popover` | `@tinycld/core/ui/popover` | Anything anchored to a trigger or a point that is not a list of commands: a date grid, a color grid, a comment thread |
| `Menu` | `@tinycld/core/ui/menu` | A list of commands: dropdowns, context menus, pickers whose rows are choices |

There is no exported `Modal`, `ModalContent`, `Menu.Portal`, `Menu.Overlay` or
`Menu.Content`. If a layout cannot be expressed with the surfaces below, the
surface is missing a feature — add it to core, do not hand-roll around it.

## Why one engine

Every overlay bug the app has shipped came from the same place: a primitive
exported the *mechanism* (a backdrop, a content box, a portal) and left the
*policy* (layout, dismissal, placement, platform presentation) to each call
site. Thirty-nine dialogs each chose their own header, width, height cap and
footer, and got them wrong in thirty-nine ways. Sixty-one menus each wrote the
same portal boilerplate while the hard parts leaked out of the engine: the
ScrollView that clipped submenus, the overlay that stole their clicks, the
slot at the wrong origin, the keyboard-opened menu with no measured trigger,
the custom keyframe that froze dialog size on web.

The engine (`core/ui/overlay/`, internal) owns:

- **The layer host.** One RN `Modal` per layer on native (full-screen, status
  bar translucent, so `measureInWindow` and the layer share a coordinate
  space); one portal into the app root on web. Layers stack in mount order, so
  a menu inside a dialog inside a dialog needs no z-index.
- **Dismissal.** Outside `pointerdown` in the capture phase, scoped to the
  surface's own subtree so a submenu or a nested popover reads as inside;
  Escape as a capture-phase key listener (react-native-web's TextInput
  swallows Escape on the way up, so a bubble-phase listener never sees it
  from inside a field); the Android back button; the backdrop on native. A
  surface opts into or out of each. One press dismisses one layer, the
  topmost.
- **The keyboard.** A Dialog or Sheet pushes the `modal` shortcut scope, so
  the screen's own shortcuts (a list's j/k) are muted while it is up. A
  Popover or Menu does NOT: a picker open on a board still yields to `j`,
  which moves the focus ring and closes it. A menu's own keys (arrows,
  Home/End, typeahead) are handled on its surface and stop there.
- **Focus.** Trap inside a dialog, restore to the trigger on close, and the
  initial focus of a surface that has an input, without the `requestAnimationFrame`
  workaround `PromptDialog` used to need.
- **Animation.** Only predefined Reanimated animations. A custom keyframe
  makes Reanimated pin the element's width and height on web after it runs,
  which is why dialogs could not grow with their content.

## Dialog

```tsx
<Dialog isOpen={isOpen} onClose={close} title="Board settings" size="sm">
    <Dialog.Body>…fields…</Dialog.Body>
    <Dialog.Footer>
        <Dialog.CancelButton onPress={close} />
        <Dialog.ActionButton label="Save" onPress={save} isDisabled={!canSave} />
    </Dialog.Footer>
</Dialog>
```

- `title` is required; `description` is one sentence under it. Pass `header`
  to replace the whole title row with your own (the file preview's toolbar).
- `size`: `sm` 360 · `md` 420 · `lg` 480 · `xl` 640 · `full`. Widths are the
  only widths; a dialog is never wider than 90% of the viewport.
- `Dialog.Body` scrolls once the dialog reaches 90% of the viewport height.
  Header and footer never move. Anything that must not scroll — a segmented
  control, a search field — goes between them as an ordinary child.
- `Dialog.Footer` is the button row. `Dialog.CancelButton` and
  `Dialog.ActionButton` (`isDestructive`, `isDisabled`, `testID`) are the
  buttons; other content is allowed (a left-aligned "Add people").
- `presentation`: `auto` (default), `dialog`, `sheet`. On the mobile
  breakpoint `auto` renders the same title, body and footer as a bottom sheet
  with a drag handle. Pin `dialog` only for something that must stay centered
  on a phone (a confirm).
- `hasCloseButton={false}` for a dialog that must be answered. Escape and the
  backdrop still dismiss.
- `ConfirmDialog` and `PromptDialog` are Dialogs; use them rather than
  building a third.

## Popover

```tsx
<Popover trigger={<DueChip due={due} />} placement="bottom-start">
    <MiniCalendar selectedDate={due} onDateSelect={pick} />
</Popover>
```

- `trigger` is cloned with `onPress` and a ref; it must be a Pressable-like
  element. Controlled use: `isOpen`, `onOpenChange`, and `anchor` — a trigger
  ref, or `{ x, y }` for a point (context menus, the calc grid).
- `placement`: `top|bottom|left|right` × `start|center|end`. The engine flips
  to the side with more room and caps height to what that side has; the
  content scrolls internally.
- On the mobile breakpoint a Popover renders as a sheet unless
  `presentation="popover"` is pinned.

Placement is a pure function, `placePopover()` in `core/ui/popover/place.ts`,
with unit tests. Do not put positioning arithmetic anywhere else.

## Menu

```tsx
<Menu trigger={<IconButton icon={MoreHorizontal} label="List actions" />} placement="bottom-end">
    <Menu.Item label="Rename list" icon={Pencil} onSelect={rename} />
    <Menu.Item label="Move left" icon={ArrowLeft} onSelect={moveLeft} isDisabled={!canMoveLeft} />
    <Menu.Sub label={`Status: ${categoryLabel(selected)}`}>
        <Menu.Item label="Backlog" leading={<CategoryGlyph category="backlog" />} isSelected onSelect={…} />
    </Menu.Sub>
    <Menu.Separator />
    <Menu.Item label="Delete list" icon={Trash2} isDestructive onSelect={confirmDelete} />
</Menu>
```

A Menu is a Popover with menu semantics: `role="menu"`, arrow keys and
typeahead, Home/End, Enter and Space, one submenu level, and items that close
the whole chain when chosen.

- `Menu.Item`: `label`, `icon` (Lucide) or `leading` (any node) or `colorDot`,
  `onSelect`, `href` (renders an anchor on web), `shortcut` (`⌘B`, shown
  right-aligned as written), `isSelected` (a check mark),
  `isDisabled`, `isDestructive`, `testID`. There is no children form: a row is
  a label, and a Sheet renders the same props as touch rows.
- `Menu.CheckboxItem`: `label`, `isChecked`, `onToggle`, `colorDot`. Does not
  close the menu.
- `Menu.Section` (`label`) groups items with a heading; `Menu.Separator`
  draws a rule.
- `Menu.Sub` (`label`, `icon`) opens on hover, click, Enter and ArrowRight,
  and closes on ArrowLeft and Escape. One level only.
- `Menu.Custom` holds a non-row child (a color grid, a search field) for the
  few menus that mix rows and widgets. Prefer a Popover when nothing in the
  surface is a row.
- Controlled use and `anchor` work as on Popover. `ContextMenu` wraps a Menu
  in the right-click (web) and long-press (native) gestures; the menubar and
  the calc grid menus are controlled Menus in the single-open registry.
- On the mobile breakpoint a Menu renders its items as a sheet. Pin
  `presentation="popover"` for a menu of two or three items beside its
  trigger, where a sheet would be heavier than the choice.

## Sheet

`Sheet` is the bottom surface the other three render on a phone. Packages use
it directly only for something that is a sheet on every breakpoint (the
mobile More menu, the notification drawer, the file picker). It replaces
`BottomDrawer`, keeps its gesture and its "rests on the tab bar" behavior, and
adds the title/body/footer structure so a Dialog rendered as a sheet looks
like a sheet, not a dialog squashed to the bottom edge.

## Testing

- `place.test.ts` pins the placement function: flip, clamp, cap, alignment,
  submenu offset.
- `overlay-dismiss.test.ts` pins the dismissal predicate: inside the surface,
  inside a nested surface, on the trigger, outside.
- `menu.test.tsx` and `dialog.test.tsx` render real surfaces under the
  react-native stub and drive them by keyboard and click.
- Every package's e2e suite is the acceptance test; the migration plan
  (`docs/plans/overlay-engine.md`) lists the specs that pin each surface.
