// Vitest stub for react-native-safe-area-context: zero insets everywhere,
// unless a test wraps its tree in `<SafeAreaProvider initialMetrics={…}>` (the
// real library's prop) to give device insets, as a notched phone would.
'use strict'

const React = require('react')
const ZERO = { top: 0, right: 0, bottom: 0, left: 0 }

const InsetsContext = React.createContext(ZERO)

function SafeAreaProvider({ initialMetrics, children }) {
    const insets = initialMetrics?.insets ?? ZERO
    return React.createElement(InsetsContext.Provider, { value: insets }, children)
}

module.exports = {
    __esModule: true,
    SafeAreaProvider,
    SafeAreaView: 'rn-safeareaview',
    useSafeAreaInsets: () => React.useContext(InsetsContext),
    useSafeAreaFrame: () => ({ x: 0, y: 0, width: 1024, height: 768 }),
    initialWindowMetrics: { insets: ZERO, frame: { x: 0, y: 0, width: 1024, height: 768 } },
}
