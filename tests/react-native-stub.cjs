'use strict'

// Stub for react-native in unit tests. Vite/Rollup cannot parse react-native's
// Flow type syntax or its CJS/ESM hybrid internals. This provides the minimal
// surface that tests reference transitively via core hooks.
const React = require('react')

// RN accepts `style` as a nested array; a DOM host element does not. Flatten
// it so a component that composes styles the RN way (`style={[a, b]}`) still
// renders under the string-tag stubs below.
function flattenStyle(style) {
    if (!style) return undefined
    if (!Array.isArray(style)) return style
    return Object.assign({}, ...style.map(flattenStyle).filter(Boolean))
}

// `onLayout` is delivered from a DOM event, the way react-native-web feeds it
// from a ResizeObserver: a test dispatches `rn-layout` with the box it wants
// on the element (see `fireLayout` in responsive-toolbar.test.tsx). The stub
// never invents a size. Pulled out of the props because React DOM would
// otherwise register a function prop on a custom element as a dead 'Layout'
// listener and the component would never hear it.
const LAYOUT_EVENT = 'rn-layout'

function host(tag) {
    const Host = React.forwardRef(function Host({ style, onLayout, ...props }, ref) {
        const node = React.useRef(null)
        const setRef = React.useCallback(
            (el) => {
                node.current = el
                if (typeof ref === 'function') ref(el)
                else if (ref) ref.current = el
            },
            [ref]
        )
        React.useLayoutEffect(() => {
            const el = node.current
            if (!el || !onLayout) return
            const handler = (e) => onLayout({ nativeEvent: { layout: e.detail } })
            el.addEventListener(LAYOUT_EVENT, handler)
            return () => el.removeEventListener(LAYOUT_EVENT, handler)
        }, [onLayout])
        return React.createElement(tag, { ...props, ref: setRef, style: flattenStyle(style) })
    })
    Host.displayName = tag
    return Host
}

// A TextInput answers DOM `input` events with onChangeText, so a test types
// the way it would into a real field: fireEvent.input(el, { target: { value } }).
// The tag is registered as a custom element with a `value` property because
// fireEvent needs a value setter, and React then sets the controlled value as
// that property. onChangeText is pulled out of the props for the same reason
// as onLayout above.
const TEXT_INPUT_TAG = 'rn-textinput'

if (typeof customElements !== 'undefined' && !customElements.get(TEXT_INPUT_TAG)) {
    customElements.define(
        TEXT_INPUT_TAG,
        class extends HTMLElement {
            get value() {
                return this._value ?? ''
            }
            set value(next) {
                this._value = next
            }
        }
    )
}

const TextHost = host(TEXT_INPUT_TAG)
const TextInput = React.forwardRef(function TextInput({ onChangeText, ...props }, ref) {
    const node = React.useRef(null)
    const setRef = React.useCallback(
        (el) => {
            node.current = el
            if (typeof ref === 'function') ref(el)
            else if (ref) ref.current = el
        },
        [ref]
    )
    React.useLayoutEffect(() => {
        const el = node.current
        if (!el || !onChangeText) return
        const handler = (e) => onChangeText(e.target.value)
        el.addEventListener('input', handler)
        return () => el.removeEventListener('input', handler)
    }, [onChangeText])
    return React.createElement(TextHost, { ...props, ref: setRef })
})
TextInput.displayName = TEXT_INPUT_TAG

// A Pressable answers a DOM click with its onPress, so a test can drive a
// button the way a user does. `disabled` swallows it, as on every platform.
const Pressable = React.forwardRef(function Pressable(
    { style, onPress, disabled, onClick, ...props },
    ref
) {
    return React.createElement('rn-pressable', {
        ...props,
        ref,
        style: flattenStyle(style),
        onClick: disabled ? undefined : (onClick ?? onPress),
    })
})
Pressable.displayName = 'rn-pressable'

module.exports = {
    Platform: { OS: 'web', select: (map) => map.web ?? map.default },
    Dimensions: {
        get: () => ({ width: 1024, height: 768 }),
        addEventListener: () => ({ remove: () => {} }),
    },
    StatusBar: {
        currentHeight: 0,
        setBarStyle: () => {},
        setBackgroundColor: () => {},
    },
    StyleSheet: { create: (s) => s, flatten: (s) => s, compose: (a, b) => [a, b] },
    // Components render as string tags so props pass straight through to
    // attributes (text-input-autofill.test.tsx asserts on them). The names are
    // dashed lowercase because React DOM treats those as custom elements and
    // stays quiet; an uppercase tag like 'Text' triggers the "is using
    // incorrect casing" / "unrecognized in this browser" dev warnings in every
    // render. The layout primitives go through `host` so an array `style`
    // survives the trip.
    View: host('rn-view'),
    Text: host('rn-text'),
    Pressable,
    ScrollView: host('rn-scrollview'),
    TextInput,
    Image: 'rn-image',
    Modal: 'rn-modal',
    TouchableOpacity: 'rn-touchableopacity',
    TouchableHighlight: 'rn-touchablehighlight',
    TouchableWithoutFeedback: 'rn-touchablewithoutfeedback',
    ActivityIndicator: 'rn-activityindicator',
    FlatList: 'rn-flatlist',
    SectionList: 'rn-sectionlist',
    SafeAreaView: 'rn-safeareaview',
    KeyboardAvoidingView: 'rn-keyboardavoidingview',
    Animated: {
        View: host('rn-view'),
        Text: host('rn-text'),
        Value: class {
            constructor(v) { this._value = v }
            setValue(v) { this._value = v }
            interpolate() { return this }
        },
        timing: () => ({ start: () => {} }),
        spring: () => ({ start: () => {} }),
        parallel: () => ({ start: () => {} }),
        sequence: () => ({ start: () => {} }),
        createAnimatedComponent: (c) => c,
        // Animated.View/Text render like their plain counterparts, so a
        // component built on react-native's own Animated (the toast) mounts.
        View: host('rn-view'),
        Text: host('rn-text'),
        ScrollView: host('rn-scrollview'),
        Image: host('rn-image'),
    },
    Easing: { linear: (t) => t, ease: (t) => t, bezier: () => (t) => t },
    PixelRatio: { get: () => 2, roundToNearestPixel: (n) => n },
    Appearance: { getColorScheme: () => 'light', addChangeListener: () => ({ remove: () => {} }) },
    I18nManager: { isRTL: false },
    Linking: { openURL: () => Promise.resolve(), canOpenURL: () => Promise.resolve(true) },
    AccessibilityInfo: {
        isScreenReaderEnabled: () => Promise.resolve(false),
        setAccessibilityFocus: () => {},
    },
    // gluestack's Modal subscribes to the keyboard for its bottom inset and
    // dismisses it on open; a dialog cannot mount under the stub without both.
    Keyboard: { addListener: () => ({ remove: () => {} }), dismiss: () => {} },
    findNodeHandle: () => null,
    useColorScheme: () => 'light',
    useWindowDimensions: () => ({ width: 1024, height: 768, scale: 1, fontScale: 1 }),
}
