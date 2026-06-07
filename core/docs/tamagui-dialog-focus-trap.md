# Tamagui Dialog Focus Trap vs Text Selection

## The Problem

When react-pdf (or any content with selectable text) is rendered inside a Tamagui `<Dialog modal>`, click-and-drag text selection does not work. The user can click, double-click to select a word, and use Cmd+A to select all — but dragging to create a selection range silently fails.

## Root Cause

Tamagui's modal Dialog wraps its content in a `FocusScope` with `trapped={true}` (hardcoded in `DialogContentModal`). The FocusScope's focus trap works by:

1. Listening for `focusin` and `focusout` events on `document`
2. When focus leaves the trap container, scheduling a `requestAnimationFrame` callback to refocus the last focused element inside the container

During a mouse drag for text selection, the browser fires focus events as the selection changes. The FocusScope detects these as "focus leaving the container" and snaps focus back via rAF, which cancels the in-progress selection.

### Why it only affects drag-selection

- **Single click**: Places the cursor — no focus change, no trap interference
- **Double-click**: Selects a word atomically — completes before the rAF fires
- **Cmd+A**: Keyboard event — no focus change involved
- **Click-and-drag**: Ongoing gesture over multiple frames — the rAF refocus fires mid-drag and kills the selection

### Why it doesn't affect text outside a Dialog

Without the Dialog's FocusScope `trapped={true}`, there is no `focusin`/`focusout` listener snapping focus back. The browser's native drag-selection works uninterrupted.

## The Fix

The `useFocusTrapDragFix` hook in `PdfCanvasViewer.tsx` suppresses `focusin`/`focusout` event propagation during active mouse drags within the PDF container. This prevents the FocusScope's trap from interfering with text selection while leaving the trap fully functional for keyboard navigation (Tab, Escape, etc.).

```tsx
function useFocusTrapDragFix(containerRef: React.RefObject<HTMLElement | null>) {
    useEffect(() => {
        const container = containerRef.current
        if (!container) return

        let isDragging = false

        const onMouseDown = (e: MouseEvent) => {
            if (container.contains(e.target as Node)) {
                isDragging = true
            }
        }
        const onMouseUp = () => {
            isDragging = false
        }
        const onFocusIn = (e: FocusEvent) => {
            if (isDragging) e.stopImmediatePropagation()
        }
        const onFocusOut = (e: FocusEvent) => {
            if (isDragging) e.stopImmediatePropagation()
        }

        document.addEventListener('mousedown', onMouseDown, { capture: true })
        document.addEventListener('mouseup', onMouseUp, { capture: true })
        document.addEventListener('focusin', onFocusIn, { capture: true })
        document.addEventListener('focusout', onFocusOut, { capture: true })

        return () => {
            document.removeEventListener('mousedown', onMouseDown, { capture: true })
            document.removeEventListener('mouseup', onMouseUp, { capture: true })
            document.removeEventListener('focusin', onFocusIn, { capture: true })
            document.removeEventListener('focusout', onFocusOut, { capture: true })
        }
    }, [containerRef])
}
```

### How it works

- **Capture phase**: All four listeners use `{ capture: true }` so they fire before the FocusScope's bubble-phase `focusin`/`focusout` listeners.
- **Scoped to container**: Only drags originating inside the PDF container trigger the suppression. Clicks on other dialog elements (buttons, close, etc.) are unaffected.
- **Minimal impact**: Only `focusin`/`focusout` are suppressed, and only during active drags. Keyboard focus trapping (Tab looping, Escape to close) continues to work normally.

### Usage

Apply the hook to any component that needs drag-selectable text inside a Tamagui modal Dialog:

```tsx
export function PdfCanvasViewer({ url }: { url: string }) {
    const containerRef = useRef<HTMLDivElement>(null)
    useFocusTrapDragFix(containerRef)

    return (
        <div ref={containerRef}>
            {/* content that needs text selection */}
        </div>
    )
}
```

## Why not use Dialog props?

- **`trapFocus`**: Not exposed. `DialogContentModal` hardcodes `trapFocus={context.open}`, and this is spread *after* `{...props}`, so it cannot be overridden.
- **`onOpenAutoFocus`**: Controls the initial focus on mount. Calling `event.preventDefault()` prevents the initial auto-focus but does NOT disable the ongoing focus trap.
- **`onInteractOutside` / `onFocusOutside`**: Control dismiss behavior when interacting outside the dialog. They don't affect the focus trap's internal refocus mechanism.

## Known upstream issues

This is a known class of problems with Tamagui's Dialog FocusScope:

- [#2118 — `trapFocus` forces element to always be focused](https://github.com/tamagui/tamagui/issues/2118): Clicking a non-focusable element inside a Dialog doesn't drop focus. Root cause identified as `display: contents` on the FocusScope wrapper causing `contains()` checks to fail, triggering false refocus.
- [#2223 — Input inside Dialog doesn't drop focus as expected](https://github.com/tamagui/tamagui/issues/2223): Calling `blur()` on an Input inside a Dialog causes the text to be re-highlighted. The maintainer confirmed this is the focus guard trapping focus and recommended disabling `trapFocus`, but also noted `trapFocus` is hardcoded and can't be set to `false` from `Dialog.Content`.
- [#2975 — Input not focusable/editable when inside a Sheet within a Dialog](https://github.com/tamagui/tamagui/issues/2975): Nested Dialog + Sheet causes the outer Dialog's focus trap to steal focus from Inputs in the inner Sheet.

All three share the same root cause: the FocusScope's `focusin`/`focusout` listeners aggressively refocus elements when they detect focus movement, even for legitimate user interactions within the dialog.

## Relevant source files

- `packages/drive/components/PdfCanvasViewer.tsx` — the fix
- `node_modules/@tamagui/focus-scope/src/FocusScope.tsx` — FocusScope implementation, `setupFocusTrap()` at line 68
- `node_modules/@tamagui/dialog/src/Dialog.tsx` — `DialogContentModal` at line 499, `DialogContentImpl` at line 618

## Debugging notes

If you encounter similar issues with other content inside Tamagui Dialogs, check:

1. Does the content need drag interaction? (text selection, sliders, canvas drawing)
2. Does the drag cause focus events? (use browser DevTools to monitor `focusin`/`focusout`)
3. If so, apply `useFocusTrapDragFix` to the drag-interactive container.
