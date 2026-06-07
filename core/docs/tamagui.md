# Tamagui Configuration

This document provides an overview of the Tamagui configuration for this project.

## Configuration Settings

**IMPORTANT:** These settings affect how you write Tamagui code in this project.

### Default Font: `body`

All text components will use the "body" font family by default.

### Only Allow Shorthands: `false`

You can use either shorthand or full property names.

### Web Container Type: `inline-size`

Enables web-specific container query optimizations.

## Shorthand Properties

These shorthand properties are available for styling:

- `b` → `bottom`
- `bg` → `backgroundColor`
- `content` → `alignContent`
- `grow` → `flexGrow`
- `items` → `alignItems`
- `justify` → `justifyContent`
- `l` → `left`
- `m` → `margin`
- `maxH` → `maxHeight`
- `maxW` → `maxWidth`
- `mb` → `marginBottom`
- `minH` → `minHeight`
- `minW` → `minWidth`
- `ml` → `marginLeft`
- `mr` → `marginRight`
- `mt` → `marginTop`
- `mx` → `marginHorizontal`
- `my` → `marginVertical`
- `p` → `padding`
- `pb` → `paddingBottom`
- `pl` → `paddingLeft`
- `pr` → `paddingRight`
- `pt` → `paddingTop`
- `px` → `paddingHorizontal`
- `py` → `paddingVertical`
- `r` → `right`
- `rounded` → `borderRadius`
- `select` → `userSelect`
- `self` → `alignSelf`
- `shrink` → `flexShrink`
- `t` → `top`
- `text` → `textAlign`
- `z` → `zIndex`

## Themes

Themes are organized hierarchically and can be combined:

**Level 1 (Base):**

- dark
- light

**Level 2 (Color Schemes):**

- accent
- black
- blue
- gray
- green
- neutral
- orange
- pink
- purple
- red
- surface1
- surface2
- teal
- white
- yellow

**Component Themes:**

- Button
- Input
- Progress
- ProgressIndicator
- Slider
- SliderActive
- SliderThumb
- Switch
- SwitchThumb
- TextArea
- Tooltip

### Theme Usage

Themes are combined hierarchically. For example, `light_blue_alt1_Button` combines:
- Base: `light`
- Color: `blue`
- Variant: `alt1`
- Component: `Button`

**Basic usage:**

```tsx
// Apply a theme to components
export default () => (
  <Theme name="dark">
    <Button>I'm a dark button</Button>
  </Theme>
)

// Themes nest and combine automatically
export default () => (
  <Theme name="dark">
    <Theme name="blue">
      <Button>Uses dark_blue theme</Button>
    </Theme>
  </Theme>
)
```

**Accessing theme values:**

Components can access theme values using `$` token syntax:

```tsx
<View backgroundColor="$background" color="$color" />
```

**Special props:**

- `inverse`: Automatically swaps light ↔ dark themes
- `reset`: Reverts to grandparent theme

## Tokens

Tokens are design system values that can be referenced using the `$` prefix.

### Space Tokens

- `-20`: -186
- `-19`: -172
- `-18`: -158
- `-17`: -144
- `-16`: -144
- `-15`: -130
- `-14`: -116
- `-13`: -102
- `-12`: -88
- `-11`: -74
- `-10`: -60
- `-9`: -53
- `-8`: -46
- `-7`: -39
- `-6`: -32
- `-5`: -24
- `-4.5`: -21
- `-4`: -18
- `-3.5`: -16
- `-3`: -13
- `-2.5`: -10
- `-2`: -7
- `-1.5`: -4
- `-1`: -2
- `-0.75`: -1.5
- `-0.5`: -1
- `-0.25`: -0.5
- `-true`: -18
- `0`: 0
- `0.25`: 0.5
- `0.5`: 1
- `0.75`: 1.5
- `1`: 2
- `1.5`: 4
- `2`: 7
- `2.5`: 10
- `3`: 13
- `3.5`: 16
- `4`: 18
- `4.5`: 21
- `5`: 24
- `6`: 32
- `7`: 39
- `8`: 46
- `9`: 53
- `10`: 60
- `11`: 74
- `12`: 88
- `13`: 102
- `14`: 116
- `15`: 130
- `16`: 144
- `17`: 144
- `18`: 158
- `19`: 172
- `20`: 186
- `true`: 18

### Size Tokens

- `0`: 0
- `0.25`: 2
- `0.5`: 4
- `0.75`: 8
- `1`: 20
- `1.5`: 24
- `2`: 28
- `2.5`: 32
- `3`: 36
- `3.5`: 40
- `4`: 44
- `4.5`: 48
- `5`: 52
- `6`: 64
- `7`: 74
- `8`: 84
- `9`: 94
- `10`: 104
- `11`: 124
- `12`: 144
- `13`: 164
- `14`: 184
- `15`: 204
- `16`: 224
- `17`: 224
- `18`: 244
- `19`: 264
- `20`: 284
- `true`: 44

### Radius Tokens

- `0`: 0
- `1`: 3
- `2`: 5
- `3`: 7
- `4`: 9
- `5`: 10
- `6`: 16
- `7`: 19
- `8`: 22
- `9`: 26
- `10`: 34
- `11`: 42
- `12`: 50
- `true`: 9

### Z-Index Tokens

- `0`: 0
- `1`: 100
- `2`: 200
- `3`: 300
- `4`: 400
- `5`: 500

### Token Usage

Tokens can be used in component props with the `$` prefix:

```tsx
// Space tokens - for margin, padding, gap
<View padding="$4" gap="$2" margin="$3" />

// Size tokens - for width, height, dimensions
<View width="$10" height="$6" />

// Color tokens - for colors and backgrounds
<View backgroundColor="$blue5" color="$gray12" />

// Radius tokens - for border-radius
<View borderRadius="$4" />
```

## Media Queries

Available responsive breakpoints:

- **height-lg**: {"minHeight":1024}
- **height-md**: {"minHeight":768}
- **height-sm**: {"minHeight":640}
- **hoverable**: {"hover":"hover"}
- **lg**: {"minWidth":1024}
- **max-100**: {"maxWidth":99.98}
- **max-200**: {"maxWidth":199.98}
- **max-height-100**: {"maxHeight":99.98}
- **max-height-200**: {"maxHeight":199.98}
- **max-height-lg**: {"maxHeight":1023.98}
- **max-height-md**: {"maxHeight":767.98}
- **max-height-sm**: {"maxHeight":639.98}
- **max-height-xs**: {"maxHeight":459.98}
- **max-height-xxs**: {"maxHeight":339.98}
- **max-height-xxxs**: {"maxHeight":259.98}
- **max-lg**: {"maxWidth":1023.98}
- **max-md**: {"maxWidth":767.98}
- **max-sm**: {"maxWidth":639.98}
- **max-xl**: {"maxWidth":1279.98}
- **max-xs**: {"maxWidth":459.98}
- **max-xxl**: {"maxWidth":1535.98}
- **max-xxs**: {"maxWidth":339.98}
- **max-xxxs**: {"maxWidth":259.98}
- **md**: {"minWidth":768}
- **sm**: {"minWidth":640}
- **touchable**: {"pointer":"coarse"}
- **xl**: {"minWidth":1280}
- **xs**: {"minWidth":460}
- **xxl**: {"minWidth":1536}
- **xxs**: {"minWidth":340}
- **xxxs**: {"minWidth":260}

### Media Query Usage

Media queries can be used as style props or with the `useMedia` hook:

```tsx
// As style props (prefix with $)
<View width="100%" $height-lg={{ width: "50%" }} />

// Using the useMedia hook
const media = useMedia()
if (media.height-lg) {
  // Render for this breakpoint
}
```

## Fonts

Available font families:

- body
- heading

## Animations

Available animation presets:



## Components

The following components are available:

- AlertDialogAction
- AlertDialogCancel
- AlertDialogDescription
- AlertDialogDestructive
- AlertDialogOverlay
- AlertDialogTitle
- AlertDialogTrigger
- Anchor
- Article
- Aside
- AvatarFallback
  - AvatarFallback.Frame
- AvatarFrame
- Button
- Card
  - Card.Background
  - Card.Footer
  - Card.Frame
  - Card.Header
- Checkbox
  - Checkbox.Frame
  - Checkbox.IndicatorFrame
- Circle
- CollapsibleContent
  - CollapsibleContent.Frame
- CollapsibleTrigger
  - CollapsibleTrigger.Frame
- DialogClose
- DialogContent
- DialogDescription
- DialogOverlay
  - DialogOverlay.Frame
- DialogPortalFrame
- DialogTitle
- DialogTrigger
- Em
- EnsureFlexed
- Fieldset
- Footer
- Form
  - Form.Frame
  - Form.Trigger
- Frame
- Group
  - Group.Frame
- H1
- H2
- H3
- H4
- H5
- H6
- Handle
- Header
- Heading
- Image
- Input
- Label
  - Label.Frame
- ListItem
- Main
- Nav
- Overlay
- Paragraph
- PopoverArrow
- PopoverContent
- PopperAnchor
- PopperArrowFrame
- PopperContentFrame
- Progress
  - Progress.Frame
  - Progress.Indicator
  - Progress.IndicatorFrame
- RadioGroup
  - RadioGroup.Frame
  - RadioGroup.IndicatorFrame
  - RadioGroup.ItemFrame
- ScrollView
- Section
- SelectGroupFrame
- SelectIcon
- SelectSeparator
- Separator
- SizableStack
- SizableText
- SliderActiveFrame
- SliderFrame
- SliderThumb
  - SliderThumb.Frame
- SliderTrackFrame
- Spacer
- Span
- Spinner
- Square
- Strong
- Switch
  - Switch.Frame
  - Switch.Thumb
- Tabs
- Text
  - Text.Area
- ThemeableStack
- Thumb
- Toast
- View
- View
- VisuallyHidden
- XGroup
- XStack
- YGroup
- YStack
- ZStack

