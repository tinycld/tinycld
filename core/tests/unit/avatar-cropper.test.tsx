// @vitest-environment happy-dom
import { cleanup, fireEvent, render } from '@testing-library/react'
import { AvatarCropper } from '@tinycld/core/components/AvatarCropper'
import { createElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

// Gesture handler is a native wrapper with no value under Node.
vi.mock('react-native-gesture-handler', () => ({
    Gesture: {
        Pan: () => ({ onBegin: () => ({ onUpdate: () => ({}) }) }),
        Pinch: () => ({ onBegin: () => ({ onUpdate: () => ({}) }) }),
        Simultaneous: () => ({}),
    },
    GestureDetector: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

// expo-image is a native module wrapper; under Node it has no value. RN's
// `transform` style is an array of single-key objects (`[{ translateX }]`) —
// react-native-web and native RN both understand that shape natively, but a
// bare DOM element only understands a CSS transform string, so the mock (not
// the component, which correctly uses the RN shape) converts it. Reused
// verbatim from avatar-component.test.tsx, which solved this same problem —
// the cropper's live preview must be checked against the exact same
// `cropToTransform` math Avatar renders from, so the mock has to expose the
// real width/height/transform the component computed, not a placeholder.
function transformToCss(transform: unknown): string | undefined {
    if (!Array.isArray(transform)) return undefined
    return transform
        .map((entry: Record<string, number>) => {
            const [key, value] = Object.entries(entry)[0] ?? []
            return key ? `${key}(${value}px)` : ''
        })
        .filter(Boolean)
        .join(' ')
}

vi.mock('expo-image', () => ({
    Image: ({ testID, style, source }: never) => {
        const props = { testID, style, source } as {
            testID?: string
            style?: Record<string, unknown>
            source?: { uri?: string }
        }
        const domStyle = {
            ...props.style,
            transform: transformToCss(props.style?.transform),
        }
        return createElement('rn-image', {
            testid: props.testID,
            src: props.source?.uri,
            style: domStyle,
        })
    },
}))

afterEach(cleanup)

function setup(initialCrop?: { x: number; y: number; zoom: number }) {
    const onCommit = vi.fn()
    const onCancel = vi.fn()
    const { container } = render(
        <AvatarCropper
            imageUri="https://example.test/a.jpg"
            initialCrop={initialCrop}
            onCommit={onCommit}
            onCancel={onCancel}
        />
    )
    const byId = (id: string) => {
        const node = container.querySelector(`[testid="${id}"]`) as HTMLElement | null
        if (!node) throw new Error(`${id} did not render`)
        return node
    }
    return { onCommit, onCancel, byId }
}

describe('AvatarCropper', () => {
    it('commits the initial crop unchanged when nothing is adjusted', () => {
        // x and y differ from each other and from the 0.5 default, so a
        // dropped or swapped axis cannot hide behind this assertion.
        const { onCommit, byId } = setup({ x: 0.25, y: 0.75, zoom: 2 })
        fireEvent.click(byId('avatar-cropper-save'))
        expect(onCommit).toHaveBeenCalledWith({ x: 0.25, y: 0.75, zoom: 2 })
    })

    it('commits the zoom the control reports', () => {
        const { onCommit, byId } = setup()
        fireEvent.change(byId('avatar-cropper-zoom'), { target: { value: '3' } })
        fireEvent.click(byId('avatar-cropper-save'))
        expect(onCommit).toHaveBeenCalledWith(expect.objectContaining({ zoom: 3 }))
    })

    it('never commits a zoom below 1', () => {
        const { onCommit, byId } = setup()
        fireEvent.change(byId('avatar-cropper-zoom'), { target: { value: '0.1' } })
        fireEvent.click(byId('avatar-cropper-save'))
        expect(onCommit).toHaveBeenCalledWith(expect.objectContaining({ zoom: 1 }))
    })

    it('never commits a zoom above 8', () => {
        const { onCommit, byId } = setup()
        fireEvent.change(byId('avatar-cropper-zoom'), { target: { value: '20' } })
        fireEvent.click(byId('avatar-cropper-save'))
        expect(onCommit).toHaveBeenCalledWith(expect.objectContaining({ zoom: 8 }))
    })

    it('calls onCancel without committing', () => {
        const { onCommit, onCancel, byId } = setup()
        fireEvent.click(byId('avatar-cropper-cancel'))
        expect(onCancel).toHaveBeenCalled()
        expect(onCommit).not.toHaveBeenCalled()
    })

    it('previews the exact transform Avatar would render for the crop', () => {
        // Non-default rect: x and y differ from each other and from 0.5, so a
        // dropped or swapped axis in the PREVIEW (as opposed to the committed
        // value, which the other tests already pin) cannot hide. Derived from
        // cropToTransform by hand: size defaults to 260, scaled = 260 * 3 =
        // 780, overflow = 780 - 260 = 520, translateX = -520 * 0.25 = -130,
        // translateY = -520 * 0.75 = -390.
        const { byId } = setup({ x: 0.25, y: 0.75, zoom: 3 })
        const image = byId('avatar-cropper-image')
        expect(image.style.width).toBe('780px')
        expect(image.style.height).toBe('780px')
        expect(image.style.transform).toBe('translateX(-130px) translateY(-390px)')
    })

    it('updates the preview transform live when the zoom control changes', () => {
        // Pins the preview to CURRENT state, not just the initial render: a
        // component that computed `transform` once from `initialCrop` and
        // never recomputed it would pass the test above but fail this one.
        // x=0.25, y=0.75, size=260: at zoom 5, scaled = 1300, overflow =
        // 1040, translateX = -1040 * 0.25 = -260, translateY = -1040 * 0.75 = -780.
        const { byId } = setup({ x: 0.25, y: 0.75, zoom: 3 })
        fireEvent.change(byId('avatar-cropper-zoom'), { target: { value: '5' } })
        const image = byId('avatar-cropper-image')
        expect(image.style.width).toBe('1300px')
        expect(image.style.height).toBe('1300px')
        expect(image.style.transform).toBe('translateX(-260px) translateY(-780px)')
    })
})
