import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import {
    LayerEscape,
    OverlayPortal,
    useLayerFocus,
    useOverlayLayer,
} from '@tinycld/core/ui/overlay'
import { Sheet } from '@tinycld/core/ui/sheet'
import { X } from 'lucide-react-native'
import { createContext, type ReactNode, type RefObject, useContext, useState } from 'react'
import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native'
import Animated, { FadeIn, FadeOut } from 'react-native-reanimated'

/**
 * The centered surface: a title row with a close button, a body that scrolls
 * once the dialog is as tall as the screen allows, and a footer whose
 * buttons stay put. On the mobile breakpoint the same parts render as a
 * Sheet. See docs/overlays.md.
 *
 *     <Dialog isOpen={isOpen} onClose={close} title="Board settings" size="sm">
 *         <Dialog.Body>…fields…</Dialog.Body>
 *         <Dialog.Footer>
 *             <Dialog.CancelButton onPress={close} />
 *             <Dialog.ActionButton label="Save" onPress={save} isDisabled={!canSave} />
 *         </Dialog.Footer>
 *     </Dialog>
 *
 * Anything that must NOT scroll with the body — a segmented control, a
 * search field — goes between the two as an ordinary child.
 */
export type DialogSize = 'sm' | 'md' | 'lg' | 'xl' | 'full'
export type DialogPresentation = 'auto' | 'dialog' | 'sheet'

export interface DialogProps {
    isOpen: boolean
    onClose: () => void
    title: string
    /** One line under the title, for a dialog whose purpose needs a sentence. */
    description?: string
    /**
     * Replaces the whole title row; `null` renders no header at all (a
     * lightbox whose content carries its own toolbar). The dialog still closes
     * on Escape and the backdrop.
     */
    header?: ReactNode
    /** sm 360 · md 420 · lg 480 · xl 640 · full (95% of the viewport, 90% tall). */
    size?: DialogSize
    /** `auto` is a sheet on the mobile breakpoint. Pin `dialog` for a confirm that must stay centered. */
    presentation?: DialogPresentation
    /** On the content box, which is what an e2e test scopes to. */
    testID?: string
    /** Off for a dialog that must be answered — a confirm — rather than dismissed. */
    hasCloseButton?: boolean
    /** Focused on open instead of the first focusable element. */
    initialFocusRef?: RefObject<{ focus: () => void } | null>
    children: ReactNode
}

// Whether the surface is a sheet, for the footer's safe-area padding.
const DialogPresentationContext = createContext<'dialog' | 'sheet'>('dialog')

const SIZE_CLASS: Record<DialogSize, string> = {
    sm: 'w-[90%] max-w-[360px] max-h-[90%]',
    md: 'w-[90%] max-w-[420px] max-h-[90%]',
    lg: 'w-[90%] max-w-[480px] max-h-[90%]',
    xl: 'w-[90%] max-w-[640px] max-h-[90%]',
    full: 'w-[95%] max-w-[1400px] h-[90%]',
}
// A `full` dialog pinned to the dialog presentation on a phone is the whole
// screen: a lightbox has no use for a windowed frame there.
const FULL_MOBILE_CLASS = 'w-full h-full rounded-none border-0'

function DialogRoot({
    isOpen,
    onClose,
    title,
    description,
    header,
    size = 'sm',
    presentation = 'auto',
    testID,
    hasCloseButton = true,
    initialFocusRef,
    children,
}: DialogProps) {
    const isMobile = useBreakpoint() === 'mobile'
    const isSheet = presentation === 'sheet' || (presentation === 'auto' && isMobile)

    if (isSheet) {
        return (
            <DialogPresentationContext.Provider value="sheet">
                <Sheet
                    isOpen={isOpen}
                    onClose={onClose}
                    title={header ? undefined : title}
                    description={description}
                    testID={testID}
                >
                    {header}
                    {children}
                </Sheet>
            </DialogPresentationContext.Provider>
        )
    }
    if (!isOpen) return null
    return (
        <DialogLayer
            onClose={onClose}
            sizeClass={size === 'full' && isMobile ? FULL_MOBILE_CLASS : SIZE_CLASS[size]}
            testID={testID}
            initialFocusRef={initialFocusRef}
            header={
                header === undefined ? (
                    <DialogHeader
                        title={title}
                        description={description}
                        onClose={onClose}
                        hasCloseButton={hasCloseButton}
                    />
                ) : (
                    header
                )
            }
        >
            {children}
        </DialogLayer>
    )
}

function DialogLayer({
    onClose,
    sizeClass,
    testID,
    initialFocusRef,
    header,
    children,
}: {
    onClose: () => void
    sizeClass: string
    testID?: string
    initialFocusRef?: DialogProps['initialFocusRef']
    header: ReactNode
    children: ReactNode
}) {
    const backdropColor = useThemeColor('overlay-backdrop')
    // State from a callback ref: the surface mounts inside the host a commit
    // after this component does, and the focus effect must run then.
    const [surfaceNode, setSurfaceNode] = useState<View | null>(null)

    // The backdrop covers the host, so an outside press is a backdrop press;
    // the layer joins the stack for the Android back button and for Escape.
    useOverlayLayer({
        isOpen: true,
        nodes: () => [surfaceNode as unknown as Node | null],
        onDismiss: onClose,
        dismissOnOutside: false,
    })
    useLayerFocus({
        isActive: true,
        container: surfaceNode as unknown as HTMLElement | null,
        initialFocusRef,
    })

    return (
        <OverlayPortal>
            <LayerEscape onEscape={onClose} />
            <View style={StyleSheet.absoluteFill} className="items-center justify-center">
                <Animated.View
                    entering={FadeIn.duration(150)}
                    exiting={FadeOut.duration(150)}
                    style={StyleSheet.absoluteFill}
                >
                    {/* Not in the accessibility tree: the header's Close button is the
                        one "Close" a reader (or a test) should find. */}
                    <Pressable
                        style={[StyleSheet.absoluteFill, { backgroundColor: backdropColor }]}
                        onPress={onClose}
                        accessible={false}
                        importantForAccessibility="no"
                    />
                </Animated.View>
                {/* A PREDEFINED entering animation, on purpose: a custom keyframe
                    (`ZoomIn.withInitialValues`) makes Reanimated pin the box's
                    width and height on web once it has run, after which the
                    dialog can never grow with its content. */}
                <Animated.View
                    ref={setSurfaceNode}
                    entering={FadeIn.duration(150)}
                    testID={testID}
                    role="dialog"
                    aria-modal
                    accessibilityViewIsModal
                    tabIndex={-1}
                    pointerEvents="auto"
                    className={`bg-background rounded-md overflow-hidden border border-border/80 shadow-hard-2 ${sizeClass}`}
                >
                    {header}
                    {children}
                </Animated.View>
            </View>
        </OverlayPortal>
    )
}

function DialogHeader({
    title,
    description,
    onClose,
    hasCloseButton,
}: {
    title: string
    description?: string
    onClose: () => void
    hasCloseButton: boolean
}) {
    return (
        <View className="flex-row items-start gap-3 px-5 pt-5 pb-3">
            <View className="flex-1 gap-1">
                <Text className="text-[16px] font-semibold text-foreground">{title}</Text>
                <DialogDescription text={description} />
            </View>
            <DialogCloseButton isVisible={hasCloseButton} onPress={onClose} />
        </View>
    )
}

function DialogDescription({ text }: { text?: string }) {
    if (!text) return null
    return <Text className="text-[13px] text-muted">{text}</Text>
}

function DialogCloseButton({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    const mutedColor = useThemeColor('muted')
    if (!isVisible) return null
    return (
        <Pressable
            accessibilityRole="button"
            accessibilityLabel="Close"
            onPress={onPress}
            hitSlop={8}
            className="rounded-md p-0.5 -mr-1 -mt-0.5 hover:bg-foreground/10 web:outline-none web:focus-visible:ring-2 web:focus-visible:ring-ring"
        >
            <X size={16} color={mutedColor} strokeWidth={2.2} />
        </Pressable>
    )
}

interface DialogBodyProps {
    children: ReactNode
    /** Replaces the default horizontal padding + row gap on the scrolled content. */
    contentClassName?: string
    testID?: string
}

/**
 * The part that scrolls. `shrink` with no `grow` is the whole trick: the body
 * is exactly as tall as its content until the dialog's cap bites, and only
 * then does it give way and scroll — the header and footer never move. A
 * `full` dialog is the exception: its body fills the fixed height.
 */
function DialogBody({ children, contentClassName, testID }: DialogBodyProps) {
    return (
        <ScrollView
            testID={testID}
            bounces={false}
            keyboardShouldPersistTaps="handled"
            className="grow-0 shrink"
            contentContainerClassName={contentClassName ?? 'px-5 pb-5 gap-3'}
        >
            {children}
        </ScrollView>
    )
}

/** The button row. Pinned below the body, so it stays reachable however long the body gets. */
function DialogFooter({ children, className }: { children: ReactNode; className?: string }) {
    const presentation = useContext(DialogPresentationContext)
    if (presentation === 'sheet')
        return <Sheet.Footer className={className}>{children}</Sheet.Footer>
    return (
        <View
            className={`flex-row justify-end items-center gap-2 px-5 py-3 border-t border-border ${className ?? ''}`}
        >
            {children}
        </View>
    )
}

interface DialogCancelButtonProps {
    onPress: () => void
    label?: string
    isDisabled?: boolean
    testID?: string
}

/** The quiet dismiss beside the action — text only, so the primary button reads as the answer. */
function DialogCancelButton({
    onPress,
    label = 'Cancel',
    isDisabled = false,
    testID,
}: DialogCancelButtonProps) {
    return (
        <Button onPress={onPress} isDisabled={isDisabled} size="sm" variant="ghost" testID={testID}>
            <ButtonText>{label}</ButtonText>
        </Button>
    )
}

interface DialogActionButtonProps {
    label: string
    onPress: () => void
    isDisabled?: boolean
    isDestructive?: boolean
    testID?: string
}

function DialogActionButton({
    label,
    onPress,
    isDisabled = false,
    isDestructive = false,
    testID,
}: DialogActionButtonProps) {
    return (
        <Button
            onPress={onPress}
            isDisabled={isDisabled}
            size="sm"
            variant={isDestructive ? 'destructive' : 'default'}
            testID={testID}
        >
            <ButtonText>{label}</ButtonText>
        </Button>
    )
}

const Dialog = Object.assign(DialogRoot, {
    Body: DialogBody,
    Footer: DialogFooter,
    CancelButton: DialogCancelButton,
    ActionButton: DialogActionButton,
})

export { Dialog }
