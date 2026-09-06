import { railWidth } from '@tinycld/core/components/workspace/rail-width'
import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { type Toast as ToastType, useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useDeviceInsets } from '@tinycld/core/lib/use-safe-area'
import { AlertTriangle, CheckCircle, Info, X, XCircle } from 'lucide-react-native'
import { useEffect, useRef } from 'react'
import { Animated, Platform, Pressable, Text, View, type ViewStyle } from 'react-native'

type ToastEdge = 'top' | 'bottom'

// Where the stack is pinned. The container is `box-none`, but each card is an
// opaque surface (it carries a dismiss button), so for a card's lifetime —
// 4–8s — nothing underneath it is clickable. That makes placement the whole
// question: it must be a region no package puts time-critical controls in.
//
// Desktop/tablet web goes bottom-left, clear of the rail. Top-right was where
// every package's header action cluster lives, so a toast raised by an action
// landed exactly on the controls a user reaches for next (boards' 6s "Sprint
// completed" card sat on the sprint-scope pill). Bottom-right is mail's compose
// window. Bottom-left past the rail covers only the tail of a sidebar list.
// Native and mobile-web keep the banner-style top placement: a bottom stack
// there would sit on the mobile tab bar.
function useToastPlacement(): { edge: ToastEdge; style: ViewStyle } {
    const insets = useDeviceInsets()
    const breakpoint = useBreakpoint()

    if (Platform.OS !== 'web') {
        // Toasts sit at the app root, above every layout that insets its own
        // content, so they must clear the sensor housing themselves — 8pt is
        // well inside a landscape inset. Same max()-not-add rule as
        // useSafeAreaPadding (these are `left`/`right` offsets rather than
        // padding, so they cannot use the hook directly).
        return {
            edge: 'top',
            style: { top: 60, right: Math.max(8, insets.right), left: Math.max(8, insets.left) },
        }
    }
    if (breakpoint === 'mobile') {
        return { edge: 'top', style: { top: 16, right: 16, width: 360 } }
    }
    return { edge: 'bottom', style: { bottom: 16, left: railWidth(insets.left) + 16, width: 360 } }
}

export function ToastRenderer() {
    const toasts = useToastStore(s => s.toasts)
    // Before the early return — hooks cannot be called conditionally.
    const placement = useToastPlacement()

    if (toasts.length === 0) return null

    return (
        <View
            testID="toast-stack"
            style={{ position: 'absolute', zIndex: 10000, gap: 8, ...placement.style }}
            pointerEvents="box-none"
        >
            {toasts.map(toast => (
                <ToastCard key={toast.id} toast={toast} edge={placement.edge} />
            ))}
        </View>
    )
}

const VARIANT_COLORS = {
    info: 'accent',
    success: 'primary',
    warning: 'warning',
    error: 'danger',
} as const

const VARIANT_ICONS = {
    info: Info,
    success: CheckCircle,
    warning: AlertTriangle,
    error: XCircle,
} as const

function ToastCard({ toast, edge }: { toast: ToastType; edge: ToastEdge }) {
    const removeToast = useToastStore(s => s.removeToast)
    const bgColor = useThemeColor('surface-secondary')
    const borderColor = useThemeColor('border')
    const mutedColor = useThemeColor('muted-foreground')
    const variantColor = useThemeColor(VARIANT_COLORS[toast.variant])

    const opacity = useRef(new Animated.Value(0)).current
    // Slide in from the edge the stack is pinned to.
    const translateY = useRef(new Animated.Value(edge === 'top' ? -20 : 20)).current

    useEffect(() => {
        Animated.parallel([
            Animated.timing(opacity, {
                toValue: 1,
                duration: 200,
                useNativeDriver: true,
            }),
            Animated.timing(translateY, {
                toValue: 0,
                duration: 200,
                useNativeDriver: true,
            }),
        ]).start()

        const timer = setTimeout(() => {
            Animated.timing(opacity, {
                toValue: 0,
                duration: 150,
                useNativeDriver: true,
            }).start(() => removeToast(toast.id))
        }, toast.duration)

        return () => clearTimeout(timer)
    }, [opacity, translateY, toast.id, toast.duration, removeToast])

    const Icon = VARIANT_ICONS[toast.variant]

    return (
        <Animated.View
            style={{
                opacity,
                transform: [{ translateY }],
                backgroundColor: bgColor,
                borderColor,
                borderWidth: 1,
                borderRadius: 12,
                padding: 14,
                flexDirection: 'row',
                alignItems: 'flex-start',
                gap: 10,
                ...(Platform.OS === 'web'
                    ? { boxShadow: '0 4px 16px rgba(0,0,0,0.12)' }
                    : {
                          elevation: 6,
                          shadowColor: '#000',
                          shadowOffset: { width: 0, height: 4 },
                          shadowOpacity: 0.12,
                          shadowRadius: 12,
                      }),
            }}
        >
            <Icon size={18} color={variantColor} style={{ marginTop: 1 }} />

            <View style={{ flex: 1, gap: 2 }}>
                <Text className="text-sm font-semibold text-foreground">{toast.title}</Text>
                <ToastBody isVisible={!!toast.body} body={toast.body} color={mutedColor} />
                <ToastAction action={toast.action} color={variantColor} />
            </View>

            <Pressable onPress={() => removeToast(toast.id)} hitSlop={8}>
                <X size={16} color={mutedColor} />
            </Pressable>
        </Animated.View>
    )
}

function ToastBody({
    isVisible,
    body,
    color,
}: {
    isVisible: boolean
    body?: string
    color: string
}) {
    if (!isVisible) return null
    return <Text style={{ fontSize: 13, color }}>{body}</Text>
}

function ToastAction({
    action,
    color,
}: {
    action?: { label: string; onPress: () => void }
    color: string
}) {
    if (!action) return null
    return (
        <Pressable onPress={action.onPress} style={{ marginTop: 4 }}>
            <Text style={{ fontSize: 13, fontWeight: '600', color }}>{action.label}</Text>
        </Pressable>
    )
}
