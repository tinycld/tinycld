// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { Avatar } from '@tinycld/core/components/Avatar'
import { createElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

// expo-image is a native module wrapper; under Node it has no value. The
// component only needs an <img>-alike that forwards testID and style. RN's
// `transform` style is an array of single-key objects (`[{ translateX }]`) —
// react-native-web and native RN both understand that shape natively, but a
// bare DOM <img> only understands a CSS transform string, so the mock (not
// the component, which correctly uses the RN shape) converts it.
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

// Rendered as a custom element (lowercase tag with a dash), the same trick
// react-native-stub.cjs uses for RN host components: React DOM permits
// arbitrary attributes on a custom element without complaint, whereas a real
// `<img>` is typed and rejects a lowercase `testid` prop.
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

function renderAvatar(element: React.ReactElement) {
    const { container } = render(element)
    const root = container.querySelector('[testid="av"]') as HTMLElement | null
    if (!root) throw new Error('avatar did not render')
    const image = container.querySelector('[testid="av-image"]') as HTMLElement | null
    return { root, image, container }
}

describe('Avatar precedence', () => {
    it('renders the image when one is supplied, over emoji and initials', () => {
        const { root, image } = renderAvatar(
            <Avatar
                name="Ada Lovelace"
                emoji="🦖"
                avatar={{ fileUrl: 'https://example.test/a.jpg' }}
                testID="av"
            />
        )
        expect(image).not.toBeNull()
        expect(root.textContent).not.toContain('🦖')
        expect(root.textContent).not.toContain('AL')
    })

    it('renders the emoji when there is no image', () => {
        const { root, image } = renderAvatar(<Avatar name="Ada Lovelace" emoji="🦖" testID="av" />)
        expect(image).toBeNull()
        expect(root.textContent).toContain('🦖')
        expect(root.textContent).not.toContain('AL')
    })

    it('falls back to two-letter initials', () => {
        const { root } = renderAvatar(<Avatar name="Ada Lovelace" testID="av" />)
        expect(root.textContent).toContain('AL')
    })

    it('derives initials from the email when the name is blank', () => {
        const { root } = renderAvatar(
            <Avatar name="" email="ada.lovelace@example.com" testID="av" />
        )
        expect(root.textContent).toContain('AL')
    })
})

describe('Avatar presentation', () => {
    it('is a full circle by default', () => {
        const { root } = renderAvatar(<Avatar name="Ada Lovelace" size={40} testID="av" />)
        expect(root.style.borderRadius).toBe('20px')
    })

    it('uses a squircle radius when asked', () => {
        const { root } = renderAvatar(
            <Avatar name="Ada Lovelace" size={40} shape="squircle" testID="av" />
        )
        // 40 * 0.32
        expect(root.style.borderRadius).toBe('12.8px')
    })

    it('honors an explicit color override, as presence does', () => {
        const { root } = renderAvatar(<Avatar name="Ada" color="#123456" testID="av" />)
        // The react-native stub passes color values through verbatim (no RNW
        // rgb() normalization) — see dialog.test.tsx for the same convention.
        expect(root.style.backgroundColor).toBe('#123456')
    })

    it('applies the crop transform to the image', () => {
        const { image } = renderAvatar(
            <Avatar
                name="Ada"
                size={100}
                avatar={{ fileUrl: 'https://example.test/a.jpg', crop: { x: 1, y: 0.5, zoom: 2 } }}
                testID="av"
            />
        )
        if (!image) throw new Error('avatar image did not render')
        expect(image.style.width).toBe('200px')
        expect(image.style.height).toBe('200px')
        expect(image.style.transform).toContain('-100px')
    })

    it('exposes the name to assistive technology', () => {
        const { root } = renderAvatar(<Avatar name="Ada Lovelace" testID="av" />)
        // The stub renders accessibilityLabel as a plain attribute, not
        // aria-label — see dialog.test.tsx's CLOSE_SELECTOR comment.
        expect(root.getAttribute('accessibilitylabel')).toBe('Ada Lovelace')
    })
})
