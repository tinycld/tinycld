// Vitest stub for react-native-reanimated.
// The real package initializes native TurboModules at import time which fails
// in a Node test environment. This stub exposes the minimal surface used by
// unit-test import chains (Animated components, animation primitives).
'use strict'

const React = require('react')
const noop = () => {}

const noopAnimation = { start: noop, stop: noop, reset: noop }

class AnimatedValue {
    constructor(value) { this._value = value }
    setValue(value) { this._value = value }
    interpolate() { return this }
}

// Host tags matching the react-native stub, so an Animated.View renders like
// a View and a test can still find what is inside it.
function host(tag) {
    const Host = React.forwardRef(function Host({ entering, exiting, layout, style, ...props }, ref) {
        const flat = Array.isArray(style) ? Object.assign({}, ...style.flat(Infinity).filter(Boolean)) : style
        return React.createElement(tag, { ...props, ref, style: flat })
    })
    Host.displayName = tag
    return Host
}

const Animated = {
    View: host('rn-view'),
    Text: host('rn-text'),
    Image: host('rn-image'),
    ScrollView: host('rn-scrollview'),
    Value: AnimatedValue,
    createAnimatedComponent: (Component) => Component,
    timing: () => noopAnimation,
    spring: () => noopAnimation,
    decay: () => noopAnimation,
    parallel: () => noopAnimation,
    sequence: () => noopAnimation,
    delay: () => noopAnimation,
}

function useSharedValue(initial) {
    return { value: initial }
}

function useAnimatedStyle() {
    return {}
}

function withSpring(value) { return value }
function withTiming(value) { return value }
function withDelay(_, animation) { return animation }
function withRepeat(animation) { return animation }
function withSequence(...animations) { return animations[0] }

// The layout-animation builders chain (`FadeIn.duration(200).easing(...)`),
// so each modifier hands back the same builder.
function animationBuilder() {
    const builder = {}
    for (const modifier of ['duration', 'delay', 'easing', 'withInitialValues', 'withCallback']) {
        builder[modifier] = () => builder
    }
    return builder
}
const FadeIn = animationBuilder()
const FadeOut = animationBuilder()
const ZoomIn = animationBuilder()
const ZoomOut = animationBuilder()
const SlideInUp = animationBuilder()
const SlideOutDown = animationBuilder()
const Easing = {
    linear: (t) => t,
    ease: (t) => t,
    bezier: () => (t) => t,
    in: (f) => f,
    out: (f) => f,
    inOut: (f) => f,
}

function runOnJS(fn) { return fn }
function runOnUI(fn) { return fn }
function useAnimatedGestureHandler() { return {} }
function useAnimatedRef() { return { current: null } }
function useAnimatedScrollHandler() { return noop }
function useDerivedValue(fn) { return { value: fn() } }
function useAnimatedReaction() {}
function cancelAnimation() {}
function makeMutable(value) { return { value } }

module.exports = {
    __esModule: true,
    default: Animated,
    Animated,
    View: Animated.View,
    Text: Animated.Text,
    Image: Animated.Image,
    ScrollView: Animated.ScrollView,
    // Also surface createAnimatedComponent at top level for any
    // import Animated from 'react-native-reanimated' that gets the full object
    createAnimatedComponent: Animated.createAnimatedComponent,
    useSharedValue,
    useAnimatedStyle,
    withSpring,
    withTiming,
    withDelay,
    withRepeat,
    withSequence,
    FadeIn,
    FadeOut,
    ZoomIn,
    ZoomOut,
    SlideInUp,
    SlideOutDown,
    Easing,
    runOnJS,
    runOnUI,
    useAnimatedGestureHandler,
    useAnimatedRef,
    useAnimatedScrollHandler,
    useDerivedValue,
    useAnimatedReaction,
    cancelAnimation,
    makeMutable,
    interpolate: (v) => v,
    Extrapolation: { CLAMP: 'CLAMP', EXTEND: 'EXTEND', IDENTITY: 'IDENTITY' },
}
