'use client'
import { createModal as createDrawer } from '@gluestack-ui/core/modal/creator'
import { ExitAnimationContext } from '@gluestack-ui/core/overlay/creator'
import type { VariantProps } from '@gluestack-ui/utils/nativewind-utils'
import { tva, useStyleContext, withStyleContext } from '@gluestack-ui/utils/nativewind-utils'
import React from 'react'
import { Pressable, ScrollView, View } from 'react-native'
import Animated, {
    Easing,
    FadeIn,
    FadeOut,
    SlideInDown,
    SlideInLeft,
    SlideInRight,
    SlideInUp,
    SlideOutDown,
    SlideOutLeft,
    SlideOutRight,
    SlideOutUp,
} from 'react-native-reanimated'

const SCOPE = 'MODAL'

const RootComponent = withStyleContext(View, SCOPE)

const AnimatedPressable = Animated.createAnimatedComponent(Pressable)
const AnimatedView = Animated.createAnimatedComponent(View)

// GlueStack's OverlayAnimatePresence drives its exit handshake via the
// react-native `Animated` API with useNativeDriver:true, whose start() callback
// never fires under react-native-web. That leaves the overlay's outer container
// mounted (display:flex) and the drawer node in the DOM AFTER onClose() runs and
// isOpen flips false — so the backdrop keeps intercepting clicks even though the
// drawer is "closed". This shim renders children as-is (our enter/exit
// animations live on Backdrop/Content via Reanimated props) and flips `exited`
// in the ExitAnimationContext whenever children go null, letting the Overlay
// unmount immediately on close. Same fix as core/ui/modal.
const AnimatePresenceShim = React.forwardRef<unknown, { children?: React.ReactNode }>(
    function AnimatePresenceShim({ children }, _ref) {
        const { setExited } = React.useContext(ExitAnimationContext)
        const isPresent = children != null
        React.useEffect(() => {
            setExited(!isPresent)
        }, [isPresent, setExited])
        return <>{children}</>
    }
)

const UIDrawer = createDrawer({
    Root: RootComponent,
    Backdrop: AnimatedPressable,
    Content: AnimatedView,
    Body: ScrollView,
    CloseButton: Pressable,
    Footer: View,
    Header: View,
    // biome-ignore lint/suspicious/noExplicitAny: GlueStack AnimatePresence type is too narrow
    AnimatePresence: AnimatePresenceShim as any,
})

const drawerStyle = tva({
    base: 'w-full h-full web:pointer-events-none relative',
    variants: {
        size: {
            sm: '',
            md: '',
            lg: '',
            full: '',
        },
        anchor: {
            left: 'items-start',
            right: 'items-end',
            top: 'justify-start',
            bottom: 'justify-end',
        },
    },
})

const drawerBackdropStyle = tva({
    base: 'absolute left-0 top-0 right-0 bottom-0 bg-[#000]/50 web:cursor-default',
})

// Drawer width is fixed at 32rem (~512px) on the side, capped at 60% of the
// viewport on narrow screens, regardless of the `size` prop. Keeping it
// consistent across view/edit/create modes prevents the content from jumping
// when the inner subview changes. Vertical drawers (top/bottom) keep height
// scaling per `size`. Use `size="full"` for an edge-to-edge drawer.
const drawerContentStyle = tva({
    base: 'bg-background shadow-hard-5 p-6 absolute',
    parentVariants: {
        size: {
            sm: '',
            md: '',
            lg: '',
            full: '',
        },
        anchor: {
            left: 'h-full border-r border-border/80 w-[32rem] max-w-[60%]',
            right: 'h-full border-l border-border/80 w-[32rem] max-w-[60%]',
            top: 'w-full border-b border-border/80 rounded-b-xl',
            bottom: 'w-full border-t border-border/80 rounded-t-xl',
        },
    },
    parentCompoundVariants: [
        {
            size: 'sm',
            anchor: 'top',
            class: 'h-1/4',
        },
        {
            size: 'sm',
            anchor: 'bottom',
            class: 'h-1/4',
        },
        {
            size: 'md',
            anchor: 'top',
            class: 'h-1/2',
        },
        {
            size: 'md',
            anchor: 'bottom',
            class: 'h-1/2',
        },
        {
            size: 'lg',
            anchor: 'top',
            class: 'h-3/4',
        },
        {
            size: 'lg',
            anchor: 'bottom',
            class: 'h-3/4',
        },
        {
            size: 'full',
            anchor: 'left',
            class: 'w-full max-w-full',
        },
        {
            size: 'full',
            anchor: 'right',
            class: 'w-full max-w-full',
        },
        {
            size: 'full',
            anchor: 'top',
            class: 'h-full',
        },
        {
            size: 'full',
            anchor: 'bottom',
            class: 'h-full',
        },
    ],
})

const drawerCloseButtonStyle = tva({
    base: 'z-10 rounded-sm p-2 data-[focus-visible=true]:bg-accent web:cursor-pointer web:outline-0 data-[hover=true]:bg-accent/50',
})

const drawerHeaderStyle = tva({
    base: 'justify-between items-center flex-row pb-4',
})

const drawerBodyStyle = tva({
    base: 'mt-4 mb-6 shrink-0',
})

const drawerFooterStyle = tva({
    base: 'flex-col-reverse gap-2 sm:flex-row sm:justify-end pt-4',
})

type IDrawerProps = React.ComponentProps<typeof UIDrawer> &
    VariantProps<typeof drawerStyle> & { className?: string }

type IDrawerBackdropProps = React.ComponentProps<typeof UIDrawer.Backdrop> &
    VariantProps<typeof drawerBackdropStyle> & { className?: string }

type IDrawerContentProps = React.ComponentProps<typeof UIDrawer.Content> &
    VariantProps<typeof drawerContentStyle> & { className?: string }

type IDrawerHeaderProps = React.ComponentProps<typeof UIDrawer.Header> &
    VariantProps<typeof drawerHeaderStyle> & { className?: string }

type IDrawerBodyProps = React.ComponentProps<typeof UIDrawer.Body> &
    VariantProps<typeof drawerBodyStyle> & { className?: string }

type IDrawerFooterProps = React.ComponentProps<typeof UIDrawer.Footer> &
    VariantProps<typeof drawerFooterStyle> & { className?: string }

type IDrawerCloseButtonProps = React.ComponentProps<typeof UIDrawer.CloseButton> &
    VariantProps<typeof drawerCloseButtonStyle> & { className?: string }

// Close the drawer on Escape while it's open. Bound directly to a
// document-level keydown listener in the CAPTURE phase (web only) rather than
// the app shortcut system: react-native-web's TextInput swallows Escape on its
// own internal handler, so the window-level (bubble-phase) shortcut matcher
// never sees it when focus is inside the drawer's inputs — capture-phase on
// document fires first. This mirrors HelpSearchPalette / FindReplaceBar, the
// overlays whose Escape-to-close is verified by e2e. Without this the only way
// to dismiss a drawer is the close button or backdrop click — an a11y gap.
function useDrawerEscape(isOpen: boolean | undefined, onClose?: () => void) {
    React.useEffect(() => {
        if (!isOpen || !onClose) return
        if (typeof document === 'undefined') return
        const onKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') {
                e.preventDefault()
                e.stopPropagation()
                onClose()
            }
        }
        document.addEventListener('keydown', onKeyDown, true)
        return () => document.removeEventListener('keydown', onKeyDown, true)
    }, [isOpen, onClose])
}

const Drawer = React.forwardRef<React.ComponentRef<typeof UIDrawer>, IDrawerProps>(function Drawer(
    { className, size = 'md', anchor = 'left', ...props },
    ref
) {
    useDrawerEscape(props.isOpen, props.onClose)

    // Don't mount the gluestack Modal at all while closed. GlueStack's exit
    // handshake runs through RN Animated with useNativeDriver:true, whose
    // start() callback never fires under react-native-web, so `exited` never
    // flips true and the Overlay's portal layer (a full-viewport absoluteFill
    // at z-9999) lingers in the DOM after onClose() — silently intercepting
    // every click in the content area behind it (Playwright sees the target as
    // unhittable; users can't click anything until a reload). Callers keep the
    // Drawer permanently mounted and just toggle `isOpen` (PivotSidePanel,
    // HelpDrawer, drive DetailPanel), so the Overlay can never unmount on its
    // own. Gating the whole tree on isOpen guarantees the overlay is gone the
    // moment the drawer closes — mirroring how PromptDialog/ConfirmDialog
    // (which return null when closed) avoid the same leak. The enter slide
    // still animates on mount; the exit slide is dropped (it was already a
    // broken no-op on web).
    if (!props.isOpen) return null

    return (
        <UIDrawer
            ref={ref}
            {...props}
            pointerEvents="box-none"
            className={drawerStyle({ size, anchor, class: className })}
            context={{ size, anchor }}
        />
    )
})

const DrawerBackdrop = React.forwardRef<
    React.ComponentRef<typeof UIDrawer.Backdrop>,
    IDrawerBackdropProps
>(function DrawerBackdrop({ className, ...props }, ref) {
    return (
        <UIDrawer.Backdrop
            ref={ref}
            entering={FadeIn.duration(200).easing(Easing.in(Easing.cubic))}
            exiting={FadeOut.duration(150)}
            {...props}
            className={drawerBackdropStyle({
                class: className,
            })}
        />
    )
})

const DrawerContent = React.forwardRef<
    React.ComponentRef<typeof UIDrawer.Content>,
    IDrawerContentProps
>(function DrawerContent({ className, ...props }, ref) {
    const { size: parentSize, anchor: parentAnchor } = useStyleContext(SCOPE)

    const customClass =
        parentAnchor === 'left' || parentAnchor === 'right'
            ? `top-0 ${parentAnchor === 'left' ? 'left-0' : 'right-0'}`
            : `left-0 ${parentAnchor === 'top' ? 'top-0' : 'bottom-0'}`

    const enteringAnimation =
        parentAnchor === 'left'
            ? SlideInLeft.duration(200).easing(Easing.in(Easing.cubic))
            : parentAnchor === 'right'
              ? SlideInRight.duration(200)
              : parentAnchor === 'top'
                ? SlideInUp.duration(200)
                : SlideInDown.duration(200)

    const exitingAnimation =
        parentAnchor === 'left'
            ? SlideOutLeft.duration(200)
            : parentAnchor === 'right'
              ? SlideOutRight.duration(200)
              : parentAnchor === 'top'
                ? SlideOutUp.duration(200)
                : SlideOutDown.duration(200)

    return (
        <UIDrawer.Content
            ref={ref}
            entering={enteringAnimation}
            exiting={exitingAnimation}
            {...props}
            className={drawerContentStyle({
                parentVariants: {
                    size: parentSize,
                    anchor: parentAnchor,
                },
                class: `${className || ''} ${customClass}`,
            })}
            pointerEvents="auto"
        />
    )
})

const DrawerHeader = React.forwardRef<
    React.ComponentRef<typeof UIDrawer.Header>,
    IDrawerHeaderProps
>(function DrawerHeader({ className, ...props }, ref) {
    return (
        <UIDrawer.Header
            ref={ref}
            {...props}
            className={drawerHeaderStyle({
                class: className,
            })}
        />
    )
})

const DrawerBody = React.forwardRef<React.ComponentRef<typeof UIDrawer.Body>, IDrawerBodyProps>(
    function DrawerBody({ className, ...props }, ref) {
        return (
            <UIDrawer.Body
                ref={ref}
                {...props}
                className={drawerBodyStyle({
                    class: className,
                })}
            />
        )
    }
)

const DrawerFooter = React.forwardRef<
    React.ComponentRef<typeof UIDrawer.Footer>,
    IDrawerFooterProps
>(function DrawerFooter({ className, ...props }, ref) {
    return (
        <UIDrawer.Footer
            ref={ref}
            {...props}
            className={drawerFooterStyle({
                class: className,
            })}
        />
    )
})

const DrawerCloseButton = React.forwardRef<
    React.ComponentRef<typeof UIDrawer.CloseButton>,
    IDrawerCloseButtonProps
>(function DrawerCloseButton({ className, ...props }, ref) {
    return (
        <UIDrawer.CloseButton
            ref={ref}
            {...props}
            className={drawerCloseButtonStyle({
                class: className,
            })}
        />
    )
})

Drawer.displayName = 'Drawer'
DrawerBackdrop.displayName = 'DrawerBackdrop'
DrawerContent.displayName = 'DrawerContent'
DrawerHeader.displayName = 'DrawerHeader'
DrawerBody.displayName = 'DrawerBody'
DrawerFooter.displayName = 'DrawerFooter'
DrawerCloseButton.displayName = 'DrawerCloseButton'

export {
    Drawer,
    DrawerBackdrop,
    DrawerBody,
    DrawerCloseButton,
    DrawerContent,
    DrawerFooter,
    DrawerHeader,
}
