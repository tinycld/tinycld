# Migration Plan: Tamagui to React Native Paper

## Context

The app currently uses HeroUI as its UI component library. We're switching to React Native Paper (MD3) to embrace full Material Design 3 aesthetics. This touches **146 files** across `components/`, `ui/`, `app/`, and 6 packages.

**Key decisions made:**
- **Layout**: Thin wrappers (`Row`/`Column`/`Box`) around RN `View` for concise migration
- **Icons**: Keep `lucide-react-native`, pass render functions to Paper components
- **Visual style**: Embrace full MD3 — use Material color generation from `#007AFF` seed
- **Menus**: Paper `Menu` + custom checkbox items (no zeego for dropdowns)

---

## Phase 0: Foundation (no screen changes)

### 0.1 Install dependencies
```
pnpm install react-native-paper @react-native-vector-icons/material-design-icons
```
Note: Expo has built-in vector icon support, but Paper requires the package for internal icon rendering.

### 0.2 Create theme — `lib/paper-theme.ts`
- Define light and dark themes extending `MD3LightTheme` / `MD3DarkTheme`
- Use Material color utilities to generate full palette from seed `#007AFF`
- Add custom color extensions for app-specific tokens not in MD3:
  - `rail`, `onRail`, `onRailActive` (addon rail)
  - `sidebar` (sidebar background)
  - `indicator` (active indicator)
  - `hover` (hover background)
- Export typed `AppTheme` and `useAppTheme()` hook
- Map Tamagui tokens to MD3 color roles:

| Tamagui Token | MD3 Color Role |
|---|---|
| `$background` | `colors.background` |
| `$color` | `colors.onBackground` |
| `$color8` | `colors.onSurfaceVariant` |
| `$placeholderColor` | `colors.onSurfaceDisabled` |
| `$borderColor` | `colors.outline` |
| `$accentBackground` | `colors.primary` |
| `$accentColor` | `colors.onPrimary` |
| `$red8` | `colors.error` |
| `$red2` | `colors.errorContainer` |
| `$overlayBackground` | `colors.backdrop` |
| `$modalOverlay` | `colors.scrim` |
| `$sidebarBackground` | `colors.surfaceContainerLow` (custom) |
| `$backgroundHover` | `colors.surfaceVariant` |

### 0.3 Create layout wrappers — `ui/layout.tsx`
Three components: `Box`, `Row`, `Column`
- Wrap RN `View` with common style props as direct props
- Resolve string color names to `theme.colors[name]` via `useAppTheme()`
- Accept numeric spacing directly (no token system)
- Props: `flex`, `gap`, `padding`, `paddingHorizontal`, `paddingVertical`, `margin`, `marginBottom`, `marginHorizontal`, `alignItems`, `justifyContent`, `backgroundColor` (string→theme color), `borderColor`, `borderWidth`, `borderRadius`, `width`, `height`, `overflow`, `opacity`, `style`
- `Row`: default `flexDirection: 'row'`
- `Column`: default `flexDirection: 'column'`
- `Box`: no default flexDirection

### 0.4 Create text wrapper — `ui/app-text.tsx`
Wraps Paper `Text` with a simpler API for migration:
- Accepts `variant` directly (Paper variants)
- Accepts `color` as string (resolved to theme color) or raw color value
- Size mapping from old Tamagui `size` values:

| Old `size=` | Paper `variant=` |
|---|---|
| `$1` | `labelSmall` |
| `$2` | `bodySmall` |
| `$3` | `bodyMedium` |
| `$4` | `bodyLarge` |
| `$5` | `titleSmall` |
| `$6` | `titleMedium` |
| `$7` | `titleLarge` |
| `$8` | `headlineSmall` |

### 0.5 Update Providers — `components/Providers.tsx`
- Add `PaperProvider` alongside `TamaguiProvider` (dual provider during migration)
- Pass light/dark theme based on `useColorScheme()`
- Both providers coexist until Phase 7 cleanup

### 0.6 Update babel config — `babel.config.cjs`
- Add `react-native-paper/babel` plugin (tree-shaking for bundle size)
- Keep `@tamagui/babel-plugin` until cleanup phase

### 0.7 Replace useBreakpoint — `components/workspace/useBreakpoint.ts`
- Replace `useMedia()` from tamagui with `useWindowDimensions()` from react-native
- Same breakpoint thresholds: lg=1024, md=768

**Files created**: `lib/paper-theme.ts`, `ui/layout.tsx`, `ui/app-text.tsx`
**Files modified**: `components/Providers.tsx`, `babel.config.cjs`, `components/workspace/useBreakpoint.ts`, `package.json`

---

## Phase 1: Core Shared Components (~15 files)

Migrate shared components that are imported across many screens. Each step is independently deployable.

| File | Key Changes |
|---|---|
| `components/ToolbarSeparator.tsx` | View → Paper `Divider` |
| `components/EmptyState.tsx` | YStack+SizableText → Column+AppText |
| `components/DataTableHeader.tsx` | XStack+SizableText → Row+AppText |
| `components/NameAvatar.tsx` | View+SizableText+useTheme → Box+AppText+useAppTheme |
| `components/LabelBadge.tsx` | XStack+SizableText → Row+AppText |
| `components/FAB.tsx` | Pressable+useTheme → Paper `FAB` |
| `components/ToolbarIconButton.tsx` | Pressable+useTheme → Paper `IconButton` |
| `components/ScreenHeader.tsx` | View → Box+Surface |
| `components/HoverAction.tsx` | useTheme → useAppTheme (`.val` → `.colors.`) |
| `components/SwipeableRow.tsx` | View+SizableText → Box+AppText |
| `components/sidebar-primitives/SidebarItem.tsx` | useTheme+View → useAppTheme+Box |
| `components/sidebar-primitives/SidebarNav.tsx` | ScrollView+View → RN ScrollView+Box |
| `components/sidebar-primitives/SidebarHeading.tsx` | SizableText → AppText |
| `components/sidebar-primitives/SidebarDivider.tsx` | Separator → Paper Divider |
| `components/sidebar-primitives/SidebarActionButton.tsx` | useTheme → Paper IconButton |

### useTheme migration pattern (90 files)
Every `useTheme()` call changes from:
```tsx
// Before (Tamagui)
const theme = useTheme()
<Icon color={theme.color8.val} />
<Icon color={theme.accentBackground.val} />

// After (Paper)
const theme = useAppTheme()
<Icon color={theme.colors.onSurfaceVariant} />
<Icon color={theme.colors.primary} />
```

---

## Phase 2: Form Components (7 files)

Critical path — all form screens depend on these.

| File | Key Changes |
|---|---|
| `ui/form/TextInput.tsx` | Tamagui `Input` → Paper `TextInput` (mode: "outlined"). YStack→Column, XStack→Row, SizableText→AppText. Paper TextInput has built-in `error` prop and `label` prop — simplifies the component. |
| `ui/form/TextAreaInput.tsx` | Tamagui `TextArea` → Paper `TextInput` with `multiline`. Same wrapper migration. |
| `ui/form/NumberInput.tsx` | Tamagui `Button` → Paper `IconButton` for +/- controls. RN TextInput stays. |
| `ui/form/SelectInput.tsx` | Tamagui `Button` options → Paper `SegmentedButtons` or `Chip` group. |
| `ui/form/Toggle.tsx` | RN `Switch` → Paper `Switch`. XStack/YStack→Row/Column. |
| `ui/form/FormErrorSummary.tsx` | YStack+SizableText → Column+Paper Text. `$red2`→`theme.colors.errorContainer`. |
| `ui/form/index.ts` | No changes needed (re-exports stay stable). |

### Paper TextInput example (replaces current `ui/form/TextInput.tsx`):
```tsx
<Paper.TextInput
  mode="outlined"
  label={label}
  value={field.value || ''}
  onChangeText={field.onChange}
  onBlur={field.onBlur}
  error={!!error}
  placeholder={placeholder}
/>
{error && <HelperText type="error">{error.message}</HelperText>}
```

---

## Phase 3: Complex Components — Dialog & Menu (~15 files)

### 3.1 DropdownMenu migration (5 files)
Replace `@tamagui/menu` with Paper `Menu`:

| File | Notes |
|---|---|
| `components/DropdownMenu.tsx` | Central abstraction. `ToolbarMenu`, `MenuActionItem`, `DotsMenu`, `MenuCheckboxItem`, `MenuSectionLabel` all rewritten using Paper Menu. CheckboxItem uses Paper Checkbox + TouchableRipple. |
| `components/workspace/UserMenu.tsx` | Uses ToolbarMenu → uses new Paper-based DropdownMenu |
| `packages/drive/sidebar.tsx` | Menu.Item → Paper Menu.Item |
| `packages/calendar/components/CalendarMenu.tsx` | Menu → Paper Menu |
| `packages/mail/sidebar.tsx` | If uses @tamagui/menu |

Key mapping:
- `Menu.Trigger asChild` → `anchor={<Pressable onPress={open}>...</Pressable>}`
- `Menu.Portal + Menu.Content` → eliminated (Paper Menu handles its own portal)
- `Menu.Item` → `Menu.Item` with `leadingIcon={() => <LucideIcon />}`
- `Menu.CheckboxItem` → custom `<TouchableRipple><Row><Checkbox /><Text /></Row></TouchableRipple>`
- `Menu.ItemTitle` → `title` prop on Menu.Item
- `Menu.Separator` → Paper `Divider`

### 3.2 Dialog migration (8 files)

| File | Notes |
|---|---|
| `components/LabelManagerDialog.tsx` | Dialog+Overlay+Content → Paper Portal+Dialog. Separator→Divider. Input→Paper TextInput. |
| `components/SuretyGuard.tsx` | Popover → Paper Dialog (confirmation pattern) |
| `packages/drive/components/ShareDialog.tsx` | Dialog → Paper Dialog |
| `packages/drive/components/ChooseFolderDialog.tsx` | Dialog → Paper Dialog |
| `packages/drive/components/PreviewModal.tsx` | Dialog → Paper Modal/Portal |
| `packages/drive/components/DetailPanel.tsx` | Dialog → Paper Dialog |
| `packages/drive/components/DriveToolbar.tsx` | Dialog → Paper Dialog |
| `packages/calendar/components/RecurrencePicker.tsx` | Dialog → Paper Dialog |

Key mapping:
- `Dialog modal open={visible}` → `<Portal><Dialog visible={visible}>`
- `Dialog.Portal + Dialog.Overlay` → eliminated (Paper Dialog has built-in backdrop)
- `Dialog.Content bordered elevate` → `Dialog.Content` (Paper handles elevation)
- `enterStyle/exitStyle` → eliminated (Paper handles animation)
- `onOpenChange` → `onDismiss`

### 3.3 ContextMenu (2 files)
- `packages/drive/components/DriveContextMenu.tsx` — Replace `@tamagui/context-menu` with Paper Menu triggered by long-press
- `packages/calendar/components/CalendarMenu.tsx` — Same approach

---

## Phase 4: Workspace Shell (~12 files)

| File | Key Changes |
|---|---|
| `components/workspace/WorkspaceLayout.tsx` | View+useTheme → Box+useAppTheme |
| `components/workspace/PackageRail.tsx` | YStack+View+useTheme → Column+Box+useAppTheme. Custom rail colors from extended theme. |
| `components/workspace/PackageSidebar.tsx` | View → Box |
| `components/workspace/MobileLayout.tsx` | View → Box |
| `components/workspace/MobileDrawer.tsx` | useTheme → useAppTheme. Reanimated animated styles stay. |
| `components/workspace/MobileTabBar.tsx` | useTheme → useAppTheme |
| `components/workspace/MoreDrawer.tsx` | useTheme → useAppTheme |
| `components/workspace/SkeletonLayout.tsx` | XStack/YStack/View → Row/Column/Box |
| `components/workspace/LoginModal.tsx` | View+YStack+Input → Box+Column+Paper TextInput+Paper Button |
| `components/workspace/WorkspaceLayoutProvider.tsx` | Minimal changes if any |
| `components/setup/SetupDashboard.tsx` | Button+YStack+SizableText → Paper Button+Column+AppText |
| `components/setup/SetupPage.tsx` | Same pattern |
| `components/setup/SuperuserLoginForm.tsx` | Input+Button → Paper TextInput+Paper Button |

---

## Phase 5: App Routes & Settings (~12 files)

| File | Key Changes |
|---|---|
| `app/_layout.tsx` | Remove tamagui-related imports |
| `app/a/[orgSlug]/_layout.tsx` | SizableText/View → AppText/Box |
| `app/a/[orgSlug]/settings/index.tsx` | Full settings screen migration |
| `app/a/[orgSlug]/settings/personal.tsx` | useTheme (4 calls) → useAppTheme |
| `app/a/[orgSlug]/settings/organization.tsx` | Standard migration |
| `app/a/[orgSlug]/settings/members.tsx` | Standard migration |
| `app/a/[orgSlug]/settings/labels.tsx` | Standard migration |
| `app/a/[orgSlug]/settings/[...section].tsx` | Standard migration |
| `app/tabs/_layout.tsx` | Standard migration |
| `app/tabs/index.tsx` | Standard migration |
| `app/tabs/profile.tsx` | Standard migration |
| `app/tabs/settings.tsx` | Standard migration |
| `app/share/[token].tsx` | Standard migration |

---

## Phase 6: Packages (~95 files)

Migrate by package, largest first. Each package can be done as a separate PR.

### 6.1 Mail (~30 files)
Key files: `EmailRow.tsx` (3 useTheme calls), `ComposeWindow.tsx`, `ComposeFields.tsx`, `SearchBar.tsx` (→ Paper Searchbar), `EmailHeader.tsx`, `InlineReply.tsx`, `RichTextEditor.tsx`, `RecipientField.tsx`, `RecipientSuggestionList.tsx` (has hoverStyle), `LabelBadge.tsx`

### 6.2 Drive (~22 files)
Key files: `ShareDialog.tsx` (done in Phase 3), `DropZone.tsx`, `Thumbnail.tsx`, `PublicSharePage.tsx`, `PublicPreviewFrame.tsx`, `UploadStatusBar.tsx`, preview components (5 files), `PdfCanvasViewer.tsx`

### 6.3 Calendar (~18 files)
Key files: `TimeGrid.tsx`, `WeekView.tsx`, `MonthView.tsx`, `MonthCell.tsx`, `EventBlock.tsx`, `EventDetailPopover.tsx` (→ Paper Dialog or Menu), `RecurrencePicker.tsx` (done in Phase 3), `MiniCalendar.tsx`, `CalendarList.tsx`, `ScheduleView.tsx`, `EventGuestList.tsx`, `CalendarColorDot.tsx`

### 6.4 Sheets (~10 files)
Key files: `SpreadsheetGrid.tsx`, `SpreadsheetToolbar.tsx`, `SheetTabs.tsx`, `FormulaBar.tsx`, `CellRenderer.tsx`, `CellEditor.tsx`, `ColumnHeader.tsx`, `RowHeader.tsx`

### 6.5 Contacts (~8 files)
Key files: `ContactRow.tsx`, `ContactForm.tsx`, `screens/index.tsx`, `screens/new.tsx`, `screens/[id].tsx`, `screens/directory.tsx`, `sidebar.tsx`

### 6.6 Docs (~7 files)
Key files: `DocumentEditor.tsx`, `DocumentToolbar.tsx`, `screens/index.tsx`, `screens/[id].tsx`, `screens/new.tsx`, `screens/_layout.tsx`

---

## Phase 7: Cleanup

1. Remove from `package.json`: `tamagui`, `@tamagui/config`, `@tamagui/babel-plugin`, `@tamagui/cli`
2. Delete `tamagui.config.ts`
3. Remove `@tamagui/babel-plugin` from `babel.config.cjs`
4. Remove `TamaguiProvider` from `components/Providers.tsx`
5. Remove tamagui type declaration (`declare module 'tamagui'`)
6. Delete `docs/tamagui-best-practices.md`, create `docs/paper-style-guide.md`
7. Update `CONTRIBUTING.md` — replace all Tamagui guidance with Paper guidance
8. Run `pnpm run checks` to verify no lingering tamagui imports
9. Run `pnpm run test:unit` to verify tests pass
10. Grep for any remaining `from 'tamagui'` or `from '@tamagui` imports

---

## Special Cases

### hoverStyle (3 files)
- `components/LabelManagerDialog.tsx` — Replace with `Pressable` + `onHoverIn`/`onHoverOut` state (web-only via Platform check)
- `packages/mail/components/RecipientSuggestionList.tsx` — Same approach
- `packages/drive/components/ShareDialog.tsx` — Same approach

### enterStyle/exitStyle (7 files)
All are Dialog.Overlay animations → eliminated. Paper Dialog handles its own fade animation.

### Button.Text compound pattern
`<Button><Button.Text>Label</Button.Text></Button>` → `<Button>Label</Button>` (Paper Button accepts string children directly)

### theme="accent" / theme="red" (sub-themes)
- `theme="accent"` on Button → `<Button mode="contained">` (uses primary color)
- `theme="red"` on Button → `<Button buttonColor={theme.colors.error} textColor={theme.colors.onError}>`
- `theme="accent"` on other components → use `theme.colors.primary` directly

### Spacing token conversion reference
| `$` token | Numeric value |
|---|---|
| `$1` | 4 |
| `$1.5` | 6 |
| `$2` | 8 |
| `$2.5` | 10 |
| `$3` | 12 |
| `$4` | 16 |
| `$5` | 20 |
| `$6` | 24 |
| `$8` | 32 |
| `$10` | 40 |

---

## Risks

| Risk | Impact | Mitigation |
|---|---|---|
| Visual regression across 146 files | High | Visual QA each phase. Compare screenshots before/after. |
| Loss of Tamagui's static CSS extraction on web | Medium | Profile web bundle size and render perf before/after. Paper uses runtime StyleSheet. |
| Paper Menu is simpler than @tamagui/menu | Medium | Custom CheckboxItem component fills the gap. |
| Dual provider during migration increases bundle | Low | Temporary. Removed in Phase 7. |
| No Popover in Paper | Low | Only 1 file uses Popover — converted to Dialog. |

---

## Verification

After each phase:
1. `pnpm run checks` (lint + typecheck)
2. `pnpm run test:unit`
3. Visual inspection on web (`pnpm run dev`) — check light and dark mode
4. Spot-check on iOS (`pnpm run ios`) if available
5. Grep for remaining tamagui imports in migrated files

After Phase 7 (complete):
1. `grep -r "from 'tamagui'" --include='*.tsx' --include='*.ts'` → 0 results
2. `grep -r "from '@tamagui" --include='*.tsx' --include='*.ts'` → 0 results
3. Full `pnpm run test:e2e` suite
4. Bundle size comparison (web build before vs after)

---

## Appendix A: Complete File Inventory

### Files importing from `'tamagui'` (146 files)

**components/ (~32 files)**
- `components/Providers.tsx` — TamaguiProvider
- `components/EmptyState.tsx` — YStack, SizableText
- `components/ToolbarSeparator.tsx` — View
- `components/FAB.tsx` — useTheme
- `components/ToolbarIconButton.tsx` — useTheme
- `components/HoverAction.tsx` — useTheme
- `components/LabelManagerDialog.tsx` — Dialog, Input, Button, ScrollView, Separator, SizableText, useTheme, View, XStack, YStack
- `components/LabelBadge.tsx` — SizableText, XStack
- `components/DataTableHeader.tsx` — SizableText, XStack
- `components/NameAvatar.tsx` — View, SizableText, useTheme
- `components/ScreenHeader.tsx` — View
- `components/SuretyGuard.tsx` — Popover
- `components/DropdownMenu.tsx` — SizableText, useTheme, View (+ @tamagui/menu)
- `components/SwipeableRow.tsx` — View, SizableText
- `components/setup/SetupDashboard.tsx` — Button, SizableText, YStack, useTheme
- `components/setup/SetupPage.tsx` — SizableText, YStack
- `components/setup/SuperuserLoginForm.tsx` — Button, Input, SizableText, YStack, useTheme
- `components/workspace/WorkspaceLayout.tsx` — View, useTheme
- `components/workspace/PackageRail.tsx` — YStack, View, useTheme
- `components/workspace/PackageSidebar.tsx` — View
- `components/workspace/MobileLayout.tsx` — View
- `components/workspace/MobileDrawer.tsx` — useTheme
- `components/workspace/MobileTabBar.tsx` — useTheme
- `components/workspace/MoreDrawer.tsx` — useTheme
- `components/workspace/SkeletonLayout.tsx` — XStack, YStack, View, useTheme
- `components/workspace/LoginModal.tsx` — View, YStack, Input, SizableText, useTheme
- `components/workspace/UserMenu.tsx` — useTheme (+ @tamagui/menu)
- `components/workspace/WorkspaceLayoutProvider.tsx` — minimal
- `components/workspace/useBreakpoint.ts` — useMedia
- `components/sidebar-primitives/SidebarItem.tsx` — useTheme, View
- `components/sidebar-primitives/SidebarNav.tsx` — ScrollView, View
- `components/sidebar-primitives/SidebarHeading.tsx` — SizableText
- `components/sidebar-primitives/SidebarDivider.tsx` — Separator
- `components/sidebar-primitives/SidebarActionButton.tsx` — useTheme

**ui/form/ (7 files)**
- `ui/form/TextInput.tsx` — Input, SizableText, useTheme, XStack, YStack
- `ui/form/TextAreaInput.tsx` — TextArea, SizableText, useTheme, XStack, YStack
- `ui/form/NumberInput.tsx` — Button, SizableText, useTheme, XStack, YStack
- `ui/form/SelectInput.tsx` — Button, SizableText, XStack, YStack
- `ui/form/Toggle.tsx` — SizableText, useTheme, XStack
- `ui/form/FormErrorSummary.tsx` — SizableText, YStack

**app/ (~12 files)**
- `app/_layout.tsx`
- `app/index.tsx`
- `app/share/[token].tsx`
- `app/tabs/_layout.tsx`
- `app/tabs/index.tsx`
- `app/tabs/profile.tsx`
- `app/tabs/settings.tsx`
- `app/a/[orgSlug]/_layout.tsx`
- `app/a/[orgSlug]/settings/index.tsx`
- `app/a/[orgSlug]/settings/personal.tsx`
- `app/a/[orgSlug]/settings/organization.tsx`
- `app/a/[orgSlug]/settings/members.tsx`
- `app/a/[orgSlug]/settings/labels.tsx`
- `app/a/[orgSlug]/settings/[...section].tsx`

**packages/mail/ (~30 files)**
- `packages/mail/sidebar.tsx`
- `packages/mail/hooks/useMailEditor.ts`
- `packages/mail/hooks/useMailSelection.ts`
- `packages/mail/hooks/useThreadListItems.ts`
- `packages/mail/screens/index.tsx`
- `packages/mail/screens/[id].tsx`
- `packages/mail/screens/_layout.tsx`
- `packages/mail/settings/provider.tsx`
- `packages/mail/settings/mailboxes.tsx`
- `packages/mail/components/AdvancedSearchDropdown.tsx`
- `packages/mail/components/AttachmentRibbon.tsx`
- `packages/mail/components/ComposeFields.tsx`
- `packages/mail/components/ComposeHeader.tsx`
- `packages/mail/components/ComposeToolbar.tsx`
- `packages/mail/components/ComposeWindow.tsx`
- `packages/mail/components/EmailAttachments.tsx`
- `packages/mail/components/EmailBody.tsx`
- `packages/mail/components/EmailDetailToolbar.tsx`
- `packages/mail/components/EmailHeader.tsx`
- `packages/mail/components/EmailListToolbar.tsx`
- `packages/mail/components/EmailRow.tsx`
- `packages/mail/components/InlineReply.tsx`
- `packages/mail/components/LabelBadge.tsx`
- `packages/mail/components/NotFoundState.tsx`
- `packages/mail/components/RecipientField.tsx`
- `packages/mail/components/RecipientSuggestionList.tsx`
- `packages/mail/components/RichTextEditor.tsx`
- `packages/mail/components/SearchBar.tsx`

**packages/drive/ (~22 files)**
- `packages/drive/sidebar.tsx`
- `packages/drive/screens/index.tsx`
- `packages/drive/screens/_layout.tsx`
- `packages/drive/components/ChooseFolderDialog.tsx`
- `packages/drive/components/DetailPanel.tsx`
- `packages/drive/components/DriveContextMenu.tsx`
- `packages/drive/components/DriveToolbar.tsx`
- `packages/drive/components/DropZone.tsx`
- `packages/drive/components/PdfCanvasViewer.tsx`
- `packages/drive/components/PreviewModal.tsx`
- `packages/drive/components/PublicPreviewFrame.tsx`
- `packages/drive/components/PublicSharePage.tsx`
- `packages/drive/components/ShareDialog.tsx`
- `packages/drive/components/Thumbnail.tsx`
- `packages/drive/components/UploadStatusBar.tsx`
- `packages/drive/components/previews/AudioPreview.tsx`
- `packages/drive/components/previews/CodePreview.tsx`
- `packages/drive/components/previews/GenericPreview.tsx`
- `packages/drive/components/previews/ImagePreview.tsx`
- `packages/drive/components/previews/PdfPreview.tsx`
- `packages/drive/components/previews/VideoPreview.tsx`

**packages/calendar/ (~18 files)**
- `packages/calendar/sidebar.tsx`
- `packages/calendar/screens/[id].tsx`
- `packages/calendar/screens/_layout.tsx`
- `packages/calendar/components/AllDayBar.tsx`
- `packages/calendar/components/CalendarColorDot.tsx`
- `packages/calendar/components/CalendarHeader.tsx`
- `packages/calendar/components/CalendarList.tsx`
- `packages/calendar/components/CalendarMenu.tsx`
- `packages/calendar/components/CurrentTimeIndicator.tsx`
- `packages/calendar/components/DayColumnHeader.tsx`
- `packages/calendar/components/DayView.tsx`
- `packages/calendar/components/EventBlock.tsx`
- `packages/calendar/components/EventDetailPopover.tsx`
- `packages/calendar/components/EventForm.tsx`
- `packages/calendar/components/EventGuestList.tsx`
- `packages/calendar/components/EventQuickCreate.tsx`
- `packages/calendar/components/MiniCalendar.tsx`
- `packages/calendar/components/MonthCell.tsx`
- `packages/calendar/components/MonthView.tsx`
- `packages/calendar/components/RecurrencePicker.tsx`
- `packages/calendar/components/ScheduleView.tsx`
- `packages/calendar/components/TimeGrid.tsx`
- `packages/calendar/components/WeekView.tsx`
- `packages/calendar/components/calendar-colors.ts`

**packages/sheets/ (~10 files)**
- `packages/sheets/screens/index.tsx`
- `packages/sheets/screens/[id].tsx`
- `packages/sheets/screens/_layout.tsx`
- `packages/sheets/components/CellEditor.tsx`
- `packages/sheets/components/CellRenderer.tsx`
- `packages/sheets/components/ColumnHeader.tsx`
- `packages/sheets/components/FormulaBar.tsx`
- `packages/sheets/components/RowHeader.tsx`
- `packages/sheets/components/SheetTabs.tsx`
- `packages/sheets/components/SpreadsheetGrid.tsx`
- `packages/sheets/components/SpreadsheetToolbar.tsx`

**packages/contacts/ (~8 files)**
- `packages/contacts/sidebar.tsx`
- `packages/contacts/screens/index.tsx`
- `packages/contacts/screens/new.tsx`
- `packages/contacts/screens/[id].tsx`
- `packages/contacts/screens/directory.tsx`
- `packages/contacts/components/ContactForm.tsx`
- `packages/contacts/components/ContactRow.tsx`

**packages/docs/ (~7 files)**
- `packages/docs/screens/index.tsx`
- `packages/docs/screens/[id].tsx`
- `packages/docs/screens/new.tsx`
- `packages/docs/screens/_layout.tsx`
- `packages/docs/hooks/useDocumentEditor.ts`
- `packages/docs/components/DocumentEditor.tsx`
- `packages/docs/components/DocumentToolbar.tsx`

---

## Appendix B: Current Tamagui Configuration (for reference)

### tamagui.config.ts (full)
```ts
import { defaultConfig } from '@tamagui/config/v5'
import { createTamagui } from 'tamagui'

const config = createTamagui({
    ...defaultConfig,
    settings: {
        ...defaultConfig.settings,
        onlyAllowShorthands: false,
    },
    themes: {
        ...defaultConfig.themes,
        light: {
            ...defaultConfig.themes.light,
            background: '#ffffff',
            backgroundHover: '#f8f9fa',
            color: '#1a1a1a',
            color8: '#666666',
            placeholderColor: '#9ca3af',
            borderColor: '#e0e0e0',
            accentBackground: '#007AFF',
            accentColor: '#ffffff',
            red8: '#dc2626',
            red2: '#fef2f2',
            red4: '#fecaca',
            railBackground: '#1a1a2e',
            railText: '#a0a0b8',
            railActiveText: '#ffffff',
            sidebarBackground: '#f3f4f6',
            activeIndicator: '#007AFF',
            hoverBackground: 'rgba(0, 0, 0, 0.05)',
            overlayBackground: 'rgba(0, 0, 0, 0.3)',
            modalOverlay: 'rgba(0, 0, 0, 0.4)',
        },
        dark: {
            ...defaultConfig.themes.dark,
            background: '#1a1a1a',
            backgroundHover: '#242424',
            color: '#e8e8e8',
            color8: '#999999',
            placeholderColor: '#6b7280',
            borderColor: '#333333',
            accentBackground: '#4da6ff',
            accentColor: '#ffffff',
            red8: '#f87171',
            red2: '#1a0a0a',
            red4: '#7f1d1d',
            railBackground: '#111118',
            railText: '#7a7a90',
            railActiveText: '#ffffff',
            sidebarBackground: '#1e1e1e',
            activeIndicator: '#4da6ff',
            hoverBackground: 'rgba(255, 255, 255, 0.06)',
            overlayBackground: 'rgba(0, 0, 0, 0.5)',
            modalOverlay: 'rgba(0, 0, 0, 0.6)',
        },
        light_accent: {
            ...defaultConfig.themes.light_accent,
            background: '#007AFF',
            backgroundHover: '#0066DD',
            backgroundPress: '#0055BB',
            color: '#ffffff',
        },
        dark_accent: {
            ...defaultConfig.themes.dark_accent,
            background: '#4da6ff',
            backgroundHover: '#3d96ef',
            backgroundPress: '#2d86df',
            color: '#ffffff',
        },
    },
})

export default config
type Conf = typeof config
declare module 'tamagui' {
    interface TamaguiCustomConfig extends Conf {}
}
```

### Providers.tsx (current)
```tsx
import '~/lib/crypto-polyfill'
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useColorScheme } from 'react-native'
import { GestureHandlerRootView } from 'react-native-gesture-handler'
import { SafeAreaProvider } from 'react-native-safe-area-context'
import { TamaguiProvider } from 'tamagui'
import { AuthProvider } from '~/lib/auth'
import { PBTSDBProvider, queryClient } from '~/lib/pocketbase'
import config from '~/tamagui.config'

export function Providers({ children }: { children: ReactNode }) {
    const colorScheme = useColorScheme()
    const defaultTheme = colorScheme === 'dark' ? 'dark' : 'light'
    return (
        <GestureHandlerRootView style={{ flex: 1 }}>
            <SafeAreaProvider>
                <TamaguiProvider config={config} defaultTheme={defaultTheme}>
                    <QueryClientProvider client={queryClient}>
                        <PBTSDBProvider>
                            <AuthProvider>{children}</AuthProvider>
                        </PBTSDBProvider>
                    </QueryClientProvider>
                </TamaguiProvider>
            </SafeAreaProvider>
        </GestureHandlerRootView>
    )
}
```

### useBreakpoint.ts (current)
```ts
import { useMedia } from 'tamagui'
export type Breakpoint = 'desktop' | 'tablet' | 'mobile'
export function useBreakpoint(): Breakpoint {
    const media = useMedia()
    if (media.lg) return 'desktop'
    if (media.md) return 'tablet'
    return 'mobile'
}
```

### babel.config.cjs (current — relevant section)
```js
plugins: [
    ['@tamagui/babel-plugin', { components: ['tamagui'], config: './tamagui.config.ts' }],
    'react-native-reanimated/plugin',
]
```

---

## Appendix C: React Native Paper Component Reference

### Installation
```
pnpm install react-native-paper react-native-safe-area-context
```
Expo users get vector icons built-in. Add `react-native-paper/babel` plugin for tree-shaking.

### PaperProvider Setup
```tsx
import { PaperProvider } from 'react-native-paper'
<PaperProvider theme={theme}><App /></PaperProvider>
```

### Theme Structure
```ts
{
  dark: boolean,
  version: 3,
  mode: 'adaptive' | 'exact',
  roundness: number,
  colors: { /* MD3 color roles */ },
  fonts: { /* typography variants */ },
  animation: { scale: number },
}
```

### MD3 Color Roles (complete)
- **Primary**: `primary`, `onPrimary`, `primaryContainer`, `onPrimaryContainer`
- **Secondary**: `secondary`, `onSecondary`, `secondaryContainer`, `onSecondaryContainer`
- **Tertiary**: `tertiary`, `onTertiary`, `tertiaryContainer`, `onTertiaryContainer`
- **Error**: `error`, `onError`, `errorContainer`, `onErrorContainer`
- **Surface**: `background`, `surface`, `surfaceVariant`, `surfaceDisabled`, `onSurface`, `onSurfaceVariant`, `onSurfaceDisabled`
- **Other**: `outline`, `outlineVariant`, `shadow`, `scrim`, `backdrop`
- **Elevation**: `elevation.level0` through `elevation.level5`

### Component Quick Reference

| Component | Key Props | Notes |
|---|---|---|
| `Text` | `variant` (displayLarge..bodySmall), `style` | 15 variants total |
| `Button` | `mode` (text/outlined/contained/elevated/contained-tonal), `icon`, `loading`, `disabled`, `compact`, `buttonColor`, `textColor`, `labelStyle`, `contentStyle` | Children = label text |
| `IconButton` | `icon`, `mode` (default/outlined/contained/contained-tonal), `size`, `selected`, `iconColor`, `containerColor` | |
| `FAB` | `icon`, `label`, `size` (small/medium/large), `mode` (flat/elevated), `variant` (primary/secondary/tertiary/surface), `customSize`, `visible` | |
| `TextInput` | `mode` (flat/outlined), `label`, `value`, `error`, `disabled`, `multiline`, `dense`, `left`/`right` (Icon/Affix), `textColor`, `outlineColor`, `activeOutlineColor` | |
| `Surface` | `elevation` (0-5), `mode` (flat/elevated) | Container with elevation |
| `Card` | `mode` (elevated/outlined/contained), `elevation`, `onPress` | Sub: `.Title`, `.Content`, `.Cover`, `.Actions` |
| `Dialog` | `visible`, `onDismiss`, `dismissable` | Must wrap in `Portal`. Sub: `.Title`, `.Content`, `.Actions`, `.ScrollArea`, `.Icon` |
| `Menu` | `visible`, `anchor`, `onDismiss`, `elevation`, `mode` | Sub: `.Item` (with `title`, `leadingIcon`, `trailingIcon`) |
| `Modal` | `visible`, `onDismiss`, `contentContainerStyle` | For custom modals |
| `Portal` | children | Renders above everything |
| `Divider` | `leftInset`, `horizontalInset`, `bold` | Replaces Tamagui Separator |
| `Chip` | `mode` (flat/outlined), `icon`, `avatar`, `selected`, `onPress`, `onClose`, `elevated`, `compact` | |
| `Searchbar` | `value`, `onChangeText`, `placeholder`, `mode` (bar/view), `icon`, `loading` | |
| `ActivityIndicator` | `animating`, `size`, `color` | Replaces Tamagui Spinner |
| `Switch` | `value`, `onValueChange`, `disabled`, `color` | |
| `Checkbox` | `status` (checked/unchecked/indeterminate), `onPress`, `disabled`, `color` | |
| `HelperText` | `type` (error/info), `visible` | For form field hints/errors |
| `Appbar` | `mode` (small/medium/large/center-aligned), `elevated` | Sub: `.Header`, `.Content`, `.Action`, `.BackAction` |
| `List.Item` | `title`, `description`, `left`, `right` | |
| `TouchableRipple` | `onPress`, `rippleColor` | Material ripple effect wrapper |
| `Tooltip` | `title`, `children` | |
| `DataTable` | — | Sub: `.Header`, `.Title`, `.Row`, `.Cell`, `.Pagination` |
| `Badge` | `size`, `visible` | |
| `Avatar.Text` | `label`, `size` | |
| `Avatar.Icon` | `icon`, `size` | |
| `Snackbar` | `visible`, `onDismiss`, `action`, `duration` | |
| `Banner` | `visible`, `actions`, `icon` | |
| `ProgressBar` | `progress`, `indeterminate`, `color` | |
| `SegmentedButtons` | `value`, `onValueChange`, `buttons` | For SelectInput replacement |

### useTheme() API
```tsx
import { useTheme } from 'react-native-paper'
const theme = useTheme()
// theme.colors.primary, theme.colors.onSurface, theme.colors.error, etc.
// theme.fonts.bodyMedium, theme.fonts.titleLarge, etc.
// theme.roundness (number)
```

### Custom typed theme hook
```tsx
import { useTheme } from 'react-native-paper'
type AppTheme = typeof lightTheme  // your custom theme
export const useAppTheme = () => useTheme<AppTheme>()
```

---

## Appendix D: Code Examples for Key Migrations

### Example 1: ui/form/TextInput.tsx migration

**Before (Tamagui):**
```tsx
import { Input, SizableText, useTheme, XStack, YStack } from 'tamagui'

<YStack gap="$1.5" marginBottom="$3">
  <SizableText size="$3" fontWeight="600" color="$color">{label}</SizableText>
  <XStack gap="$2" alignItems="center">
    <Input
      size="$4" flex={1}
      value={field.value || ''}
      onChangeText={field.onChange}
      placeholderTextColor="$placeholderColor"
      borderColor={hasError ? '$red8' : '$borderColor'}
      backgroundColor="$background"
      color="$color"
    />
    {addon}
  </XStack>
  {hasError && <SizableText size="$2" color="$red8">{error.message}</SizableText>}
</YStack>
```

**After (Paper):**
```tsx
import { TextInput as PaperTextInput, HelperText, useTheme } from 'react-native-paper'
import { View } from 'react-native'

<View style={{ marginBottom: 12 }}>
  <View style={{ flexDirection: 'row', gap: 8, alignItems: 'center' }}>
    <PaperTextInput
      mode="outlined"
      label={label}
      style={{ flex: 1 }}
      value={field.value || ''}
      onChangeText={field.onChange}
      onBlur={field.onBlur}
      error={hasError}
      placeholder={placeholder}
    />
    {addon}
  </View>
  {hint && !hasError && <HelperText type="info">{hint}</HelperText>}
  {hasError && <HelperText type="error" visible>{error.message}</HelperText>}
</View>
```

### Example 2: LabelManagerDialog migration

**Before (Tamagui):**
```tsx
<Dialog modal open={isVisible} onOpenChange={open => { if (!open) onClose() }}>
  <Dialog.Portal>
    <Dialog.Overlay opacity={0.3} enterStyle={{ opacity: 0 }} exitStyle={{ opacity: 0 }} />
    <Dialog.Content bordered elevate padding={0} width={360} maxHeight={480}>
      <XStack alignItems="center" justifyContent="space-between" paddingHorizontal="$3">
        <SizableText size="$5" fontWeight="600">Labels</SizableText>
        ...
      </XStack>
      <Separator />
      <LabelManagerPanel />
    </Dialog.Content>
  </Dialog.Portal>
</Dialog>
```

**After (Paper):**
```tsx
import { Dialog, Portal, Divider, Text } from 'react-native-paper'

<Portal>
  <Dialog visible={isVisible} onDismiss={onClose} style={{ maxWidth: 360, alignSelf: 'center' }}>
    <Dialog.Title>Labels</Dialog.Title>
    <Divider />
    <Dialog.Content style={{ paddingHorizontal: 0 }}>
      <LabelManagerPanel />
    </Dialog.Content>
  </Dialog>
</Portal>
```

### Example 3: DropdownMenu migration

**Before (Tamagui @tamagui/menu):**
```tsx
<Menu>
  <Menu.Trigger asChild><View><ToolbarIconButton icon={icon} /></View></Menu.Trigger>
  <Menu.Portal zIndex={100}>
    <Menu.Content borderRadius={8} minWidth={200} backgroundColor="$background" ...>
      <Menu.Item onSelect={onPress} gap="$2">
        <Menu.ItemIcon><Icon size={16} color={theme.color8.val} /></Menu.ItemIcon>
        <Menu.ItemTitle size="$3">{label}</Menu.ItemTitle>
      </Menu.Item>
    </Menu.Content>
  </Menu.Portal>
</Menu>
```

**After (Paper):**
```tsx
import { Menu, IconButton } from 'react-native-paper'

const [visible, setVisible] = useState(false)
<Menu
  visible={visible}
  onDismiss={() => setVisible(false)}
  anchor={<IconButton icon={() => <LucideIcon />} onPress={() => setVisible(true)} />}
>
  <Menu.Item
    leadingIcon={() => <Icon size={16} color={theme.colors.onSurfaceVariant} />}
    onPress={onPress}
    title={label}
  />
</Menu>
```

### Example 4: useBreakpoint migration

**Before:**
```tsx
import { useMedia } from 'tamagui'
export function useBreakpoint(): Breakpoint {
    const media = useMedia()
    if (media.lg) return 'desktop'
    if (media.md) return 'tablet'
    return 'mobile'
}
```

**After:**
```tsx
import { useWindowDimensions } from 'react-native'
export function useBreakpoint(): Breakpoint {
    const { width } = useWindowDimensions()
    if (width >= 1024) return 'desktop'
    if (width >= 768) return 'tablet'
    return 'mobile'
}
```

### Example 5: Button migration patterns

```tsx
// theme="accent" button
// Before: <Button theme="accent"><Button.Text>Save</Button.Text></Button>
// After:  <Button mode="contained" onPress={save}>Save</Button>

// theme="red" button
// Before: <Button size="$2" theme="red" onPress={del}>Delete</Button>
// After:  <Button mode="contained" buttonColor={theme.colors.error} textColor={theme.colors.onError} compact onPress={del}>Delete</Button>

// chromeless button
// Before: <Button size="$2" chromeless onPress={cancel}>Cancel</Button>
// After:  <Button mode="text" compact onPress={cancel}>Cancel</Button>

// default button
// Before: <Button size="$3" onPress={action}>Action</Button>
// After:  <Button mode="outlined" onPress={action}>Action</Button>
```

### Example 6: Providers migration (final state after Phase 7)
```tsx
import '~/lib/crypto-polyfill'
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useColorScheme } from 'react-native'
import { GestureHandlerRootView } from 'react-native-gesture-handler'
import { SafeAreaProvider } from 'react-native-safe-area-context'
import { PaperProvider } from 'react-native-paper'
import { AuthProvider } from '~/lib/auth'
import { PBTSDBProvider, queryClient } from '~/lib/pocketbase'
import { lightTheme, darkTheme } from '~/lib/paper-theme'

export function Providers({ children }: { children: ReactNode }) {
    const colorScheme = useColorScheme()
    const theme = colorScheme === 'dark' ? darkTheme : lightTheme
    return (
        <GestureHandlerRootView style={{ flex: 1 }}>
            <SafeAreaProvider>
                <PaperProvider theme={theme}>
                    <QueryClientProvider client={queryClient}>
                        <PBTSDBProvider>
                            <AuthProvider>{children}</AuthProvider>
                        </PBTSDBProvider>
                    </QueryClientProvider>
                </PaperProvider>
            </SafeAreaProvider>
        </GestureHandlerRootView>
    )
}
```

---

## Appendix E: Dependencies to Add/Remove

### Add
```json
{
  "react-native-paper": "^5.x",
  "react-native-safe-area-context": "already installed"
}
```
Note: `react-native-safe-area-context` is already a dependency. No need for `@react-native-vector-icons/material-design-icons` on Expo since Expo includes vector icons.

### Remove (Phase 7)
```json
{
  "tamagui": "^2.0.0-rc.36",
  "@tamagui/config": "^2.0.0-rc.36",
  "@tamagui/babel-plugin": "2.0.0-rc.36",
  "@tamagui/cli": "^2.0.0-rc.36"
}
```

### Delete files
- `tamagui.config.ts`
- `docs/tamagui-best-practices.md`
- `docs/tamagui-dialog-focus-trap.md`
- `docs/tamagui.md`

### Create files
- `lib/paper-theme.ts` — theme definitions
- `ui/layout.tsx` — Box/Row/Column wrappers
- `ui/app-text.tsx` — AppText wrapper around Paper Text
- `docs/paper-style-guide.md` — new style guide (Phase 7)
