// Vitest stub for react-native-gesture-handler, whose entry carries Flow
// syntax and native module setup. The Sheet surface (core/ui/sheet) builds
// its drag-to-dismiss on Gesture.Pan(); under test the gesture is inert and
// the detector renders its child.
'use strict'

const React = require('react')

function chain() {
    const gesture = {}
    for (const method of [
        'activeOffsetY',
        'activeOffsetX',
        'failOffsetY',
        'failOffsetX',
        'onUpdate',
        'onEnd',
        'onStart',
        'onBegin',
        'onFinalize',
        'enabled',
        'runOnJS',
        'simultaneousWithExternalGesture',
        'requireExternalGestureToFail',
    ]) {
        gesture[method] = () => gesture
    }
    return gesture
}

const Gesture = { Pan: chain, Tap: chain, LongPress: chain, Native: chain, Simultaneous: chain, Race: chain }

function GestureDetector({ children }) {
    return React.createElement(React.Fragment, null, children)
}

function GestureHandlerRootView({ children }) {
    return React.createElement(React.Fragment, null, children)
}

module.exports = { __esModule: true, Gesture, GestureDetector, GestureHandlerRootView }
