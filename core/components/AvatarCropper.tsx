import { type CropRect, clampCrop, cropToTransform, DEFAULT_CROP } from '@tinycld/core/lib/avatar'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Image } from 'expo-image'
import type { InputHTMLAttributes } from 'react'
import { useRef, useState } from 'react'
import { Platform, Pressable, Text, View } from 'react-native'
import { Gesture, GestureDetector } from 'react-native-gesture-handler'
import { runOnJS } from 'react-native-reanimated'

interface AvatarCropperProps {
    imageUri: string
    initialCrop?: CropRect
    size?: number
    onCommit: (rect: CropRect) => void
    onCancel: () => void
}

/**
 * Pan/zoom framing for an avatar, with a circular mask over a square viewport.
 *
 * Nothing is rasterized: the committed value is the same normalized rect that
 * Avatar renders from (`cropToTransform`), so what the user frames here is
 * exactly what every circle in the app shows, and re-opening restores their
 * framing exactly.
 *
 * The zoom control is not decoration — it is the accessible path for pointer
 * users with no pinch gesture and no trackpad, and the only path at all on
 * native for anyone who can't pinch.
 */
export function AvatarCropper({
    imageUri,
    initialCrop = DEFAULT_CROP,
    size = 260,
    onCommit,
    onCancel,
}: AvatarCropperProps) {
    const [crop, setCrop] = useState<CropRect>(() => clampCrop(initialCrop))
    // Snapshot of the crop at gesture start, so a pan/pinch computes its delta
    // against a fixed origin rather than the (stale, UI-thread-lagging) `crop`
    // state closed over when the gesture began.
    const gestureStart = useRef<CropRect>(crop)
    const foregroundColor = useThemeColor('foreground')

    // Gesture callbacks run on the UI thread and cannot touch React state
    // directly — `commit` is the only JS-thread bridge, crossed via
    // `runOnJS`, the pattern `core/ui/sheet/index.tsx` uses for its own pan
    // gesture. Committing every `onUpdate` frame (rather than only at
    // `onEnd`) is deliberate here, unlike the sheet's drag-threshold check:
    // the preview must visibly track the finger while dragging, and
    // `cropToTransform` is cheap enough to recompute every frame.
    const commit = (next: CropRect) => setCrop(clampCrop(next))

    const panGesture = Gesture.Pan()
        .onBegin(() => {
            gestureStart.current = crop
        })
        .onUpdate(event => {
            // Dragging the image right moves the focal point left. Travel is
            // scaled by the current zoom: at zoom 1 the image exactly covers
            // the viewport and there is no room to pan, so `size` is used as
            // the divisor floor to avoid a division by zero going to Infinity.
            const start = gestureStart.current
            const travel = size * (start.zoom - 1) || size
            runOnJS(commit)({
                x: start.x - event.translationX / travel,
                y: start.y - event.translationY / travel,
                zoom: start.zoom,
            })
        })

    const pinchGesture = Gesture.Pinch()
        .onBegin(() => {
            gestureStart.current = crop
        })
        .onUpdate(event => {
            const start = gestureStart.current
            runOnJS(commit)({ ...start, zoom: start.zoom * event.scale })
        })

    const transform = cropToTransform(crop, size)

    return (
        <View className="gap-4 items-center">
            <GestureDetector gesture={Gesture.Simultaneous(panGesture, pinchGesture)}>
                <View
                    testID="avatar-cropper-viewport"
                    className="overflow-hidden bg-surface-secondary"
                    style={{ width: size, height: size, borderRadius: size / 2 }}
                >
                    <Image
                        source={{ uri: imageUri }}
                        contentFit="cover"
                        style={{
                            width: transform.width,
                            height: transform.height,
                            transform: [
                                { translateX: transform.translateX },
                                { translateY: transform.translateY },
                            ],
                        }}
                    />
                </View>
            </GestureDetector>

            <ZoomControl
                zoom={crop.zoom}
                onZoom={zoom => setCrop(current => clampCrop({ ...current, zoom }))}
            />

            <View className="flex-row gap-3">
                <Pressable
                    testID="avatar-cropper-save"
                    onPress={() => onCommit(clampCrop(crop))}
                    className="rounded-lg px-4 py-2.5 bg-primary"
                >
                    <Text className="text-primary-foreground font-semibold">Save</Text>
                </Pressable>
                <Pressable
                    testID="avatar-cropper-cancel"
                    onPress={onCancel}
                    className="rounded-lg px-4 py-2.5 border border-border"
                >
                    <Text className="font-semibold" style={{ color: foregroundColor }}>
                        Cancel
                    </Text>
                </Pressable>
            </View>
        </View>
    )
}

/**
 * Zoom is platform-split rather than one shared control because the two
 * platforms have different "obvious" interaction: web has no pinch gesture at
 * all, so a keyboard-operable range input is both the accessible path and the
 * primary one; native gets pinch as the primary path (wired above) and these
 * buttons are the fallback for anyone who can't pinch. Both paths funnel
 * through the caller's `onZoom`, which clamps — this component never commits
 * an out-of-range value itself.
 */
function ZoomControl({ zoom, onZoom }: { zoom: number; onZoom: (zoom: number) => void }) {
    if (Platform.OS === 'web') {
        // `testid` (lowercase, no dash) rather than `data-testid` to match the
        // attribute name every other component in this codebase exposes —
        // the RN `View`/`Pressable` host elements pass `testID` straight
        // through and React DOM lowercases it on a custom element. A real
        // `<input>` has typed DOM props with no room for that attribute, so
        // it is cast the same way `PlainInput.tsx` casts a web-only style key.
        const webProps = {
            testid: 'avatar-cropper-zoom',
        } as InputHTMLAttributes<HTMLInputElement>
        return (
            <input
                {...webProps}
                aria-label="Zoom"
                type="range"
                min={1}
                max={8}
                step={0.1}
                value={zoom}
                onChange={event => onZoom(Number(event.target.value))}
                style={{ width: '100%' }}
            />
        )
    }

    return (
        <View testID="avatar-cropper-zoom" className="flex-row gap-3">
            <ZoomStep label="−" onPress={() => onZoom(zoom - 0.5)} />
            <ZoomStep label="+" onPress={() => onZoom(zoom + 0.5)} />
        </View>
    )
}

function ZoomStep({ label, onPress }: { label: string; onPress: () => void }) {
    return (
        <Pressable onPress={onPress} className="rounded-lg px-4 py-2 border border-border">
            <Text className="text-foreground font-semibold">{label}</Text>
        </Pressable>
    )
}
