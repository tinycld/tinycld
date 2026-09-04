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

function host(tag) {
    const Host = React.forwardRef(function Host({ style, ...props }, ref) {
        return React.createElement(tag, { ...props, ref, style: flattenStyle(style) })
    })
    Host.displayName = tag
    return Host
}

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
    Pressable: host('rn-pressable'),
    ScrollView: host('rn-scrollview'),
    TextInput: host('rn-textinput'),
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
    },
    Easing: { linear: (t) => t, ease: (t) => t, bezier: () => (t) => t },
    PixelRatio: { get: () => 2, roundToNearestPixel: (n) => n },
    Appearance: { getColorScheme: () => 'light', addChangeListener: () => ({ remove: () => {} }) },
    I18nManager: { isRTL: false },
    Linking: { openURL: () => Promise.resolve(), canOpenURL: () => Promise.resolve(true) },
    AccessibilityInfo: { isScreenReaderEnabled: () => Promise.resolve(false) },
    useColorScheme: () => 'light',
    useWindowDimensions: () => ({ width: 1024, height: 768, scale: 1, fontScale: 1 }),
}
