// Vitest stub for react-native-safe-area-context: zero insets everywhere.
'use strict'

const React = require('react')
const ZERO = { top: 0, right: 0, bottom: 0, left: 0 }

function SafeAreaProvider({ children }) {
    return React.createElement(React.Fragment, null, children)
}

module.exports = {
    __esModule: true,
    SafeAreaProvider,
    SafeAreaView: 'rn-safeareaview',
    useSafeAreaInsets: () => ZERO,
    useSafeAreaFrame: () => ({ x: 0, y: 0, width: 1024, height: 768 }),
    initialWindowMetrics: { insets: ZERO, frame: { x: 0, y: 0, width: 1024, height: 768 } },
}
