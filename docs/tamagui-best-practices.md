# Tamagui Best Practices

This project uses Tamagui with the babel compiler plugin enabled for static CSS extraction on web. Follow these conventions to get full benefit from the compiler and keep the codebase consistent.

## Use Tamagui Components Instead of React Native

Prefer Tamagui layout and text components over their React Native equivalents:

| Instead of (RN)        | Use (Tamagui)          |
|------------------------|------------------------|
| `View`                 | `View`, `XStack`, `YStack`, `ZStack` |
| `Text`                 | `SizableText`          |
| `ScrollView`           | Keep RN `ScrollView` (Tamagui doesn't replace it) |
| `Pressable`            | Keep RN `Pressable` (Tamagui `Button` when appropriate) |
| `TextInput`            | Keep RN `TextInput` or use `~/ui/form` components |

Tamagui components support style props directly, enabling the compiler to extract them to CSS at build time.

## Use `$token` Syntax on Tamagui Components

On Tamagui components, use token strings instead of `theme.*.val`:

```tsx
// GOOD - compiler can extract this to CSS
<View backgroundColor="$background" />
<SizableText color="$color8" />
<XStack borderColor="$borderColor" gap="$3" />

// BAD - blocks static extraction
const theme = useTheme()
<View backgroundColor={theme.background.val} />
```

## When to Keep `useTheme()`

`useTheme()` with `.val` is still needed when passing colors to non-Tamagui components:

```tsx
const theme = useTheme()

// Lucide icons require a color string
<Settings size={20} color={theme.color8.val} />

// RN Pressable uses style objects
<Pressable style={{ backgroundColor: theme.accentBackground.val }} />

// RN TextInput
<TextInput placeholderTextColor={theme.placeholderColor.val} />

// RN Switch
<Switch trackColor={{ true: theme.accentBackground.val }} />

// Color math (alpha suffixes)
style={{ backgroundColor: `${theme.activeIndicator.val}18` }}

// Reanimated animated styles
const animStyle = useAnimatedStyle(() => ({
    backgroundColor: theme.sidebarBackground.val,
}))
```

**Rule of thumb:** If the component is imported from `tamagui`, use `$token`. If it's from `react-native`, `lucide-react-native`, or `react-native-reanimated`, use `theme.*.val`.

## Do Not Use `StyleSheet.create`

All layout and styling should use Tamagui component props:

```tsx
// GOOD
<YStack flex={1} padding="$4" gap="$3" backgroundColor="$background">
    <SizableText size="$5" color="$color8">Hello</SizableText>
</YStack>

// BAD
const styles = StyleSheet.create({
    container: { flex: 1, padding: 16, gap: 12 },
})
<View style={[styles.container, { backgroundColor: theme.background.val }]}>
    <Text style={{ color: theme.color8.val }}>Hello</Text>
</View>
```

**Exceptions** where `StyleSheet` or inline `style` is acceptable:
- Platform-specific web hacks (`Platform.OS === 'web'` with `boxShadow`, `transition`, `height: '100vh'`)
- Reanimated `useAnimatedStyle` return values
- Styles on RN `Pressable`, `Link`, or `DraggableFlatList` (these don't accept Tamagui props)
- `StyleSheet.hairlineWidth` constant

## Use Full Property Names

Use full property names, not shorthands. While shorthands are configured, the full names are clearer and avoid confusion:

```tsx
// GOOD
<View paddingHorizontal="$3" paddingVertical="$2" backgroundColor="$background" />

// AVOID
<View px="$3" py="$2" bg="$background" />
```

## Responsive Breakpoints

Use `useBreakpoint()` from `~/components/workspace/useBreakpoint` for conditional JSX rendering (it uses Tamagui's `useMedia` internally):

```tsx
const breakpoint = useBreakpoint()
if (breakpoint === 'mobile') return <MobileLayout />
```

Breakpoints: `mobile` (< 768), `tablet` (768-1023), `desktop` (>= 1024).

For responsive **style** changes (not JSX branching), use Tamagui's media query props:

```tsx
<View width="100%" $lg={{ width: '50%' }} />
```

## Theme Colors

Custom theme tokens defined in `tamagui.config.ts`:

| Token                | Light           | Dark            | Usage                    |
|----------------------|-----------------|-----------------|--------------------------|
| `$background`        | `#ffffff`       | `#1a1a1a`       | Page/screen background   |
| `$backgroundHover`   | `#f8f9fa`       | `#242424`       | Hover states             |
| `$color`             | `#1a1a1a`       | `#e8e8e8`       | Primary text             |
| `$color8`            | `#666666`       | `#999999`       | Secondary/muted text     |
| `$borderColor`       | `#e0e0e0`       | `#333333`       | Borders, dividers        |
| `$accentBackground`  | `#007AFF`       | `#4da6ff`       | Primary action buttons   |
| `$accentColor`       | `#ffffff`       | `#ffffff`       | Text on accent bg        |
| `$sidebarBackground` | `#f3f4f6`       | `#1e1e1e`       | Sidebar panels           |
| `$railBackground`    | `#1a1a2e`       | `#111118`       | Package navigation rail  |
| `$activeIndicator`   | `#007AFF`       | `#4da6ff`       | Active nav indicators    |
| `$red8`, `$red2`     | error colors    | error colors    | Error states             |

## Reference Components

Well-migrated components to use as examples:
- **Trivial:** `components/ToolbarSeparator.tsx`, `components/sidebar-primitives/SidebarDivider.tsx`
- **With text:** `components/DataTableHeader.tsx`, `components/EmptyState.tsx`
- **Mixed (Tamagui + Lucide):** `components/sidebar-primitives/SidebarItem.tsx`, `components/DropdownMenu.tsx`
- **With Reanimated:** `components/workspace/MobileDrawer.tsx`
- **With Platform hacks:** `components/workspace/WorkspaceLayout.tsx`
