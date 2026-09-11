// @vitest-environment happy-dom
import { cleanup, fireEvent, render } from '@testing-library/react'
import { AvatarCropper } from '@tinycld/core/components/AvatarCropper'
import { afterEach, describe, expect, it, vi } from 'vitest'

// Gesture handler and expo-image are native wrappers with no value under Node.
vi.mock('react-native-gesture-handler', () => ({
    Gesture: {
        Pan: () => ({ onBegin: () => ({ onUpdate: () => ({}) }) }),
        Pinch: () => ({ onBegin: () => ({ onUpdate: () => ({}) }) }),
        Simultaneous: () => ({}),
    },
    GestureDetector: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))
vi.mock('expo-image', () => ({ Image: () => <img alt="" /> }))

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
})
