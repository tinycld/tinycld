// Vitest stub for react-native-svg.
// Its Fabric bindings (lib/module/fabric/*) import from deep react-native
// subpaths — react-native/Libraries/Utilities/codegenNativeComponent,
// react-native/Libraries/ReactNative/requireNativeComponent — which carry
// real react-native source (Flow `import type`) that Vite/Rollup cannot
// parse. The top-level `react-native` alias in vitest.config.ts only matches
// the bare specifier, not these subpaths, so anything that transitively
// imports react-native-svg (e.g. @gluestack-ui/core's icon creator, used by
// core/ui/button and core/ui/icon) crashes at collect time. Same failure
// mode as the existing lucide-react-native / uniwind / react-native-reanimated
// stubs — stub the whole package rather than teach Vite to parse Flow.
'use strict'

const React = require('react')

// A no-op component is sufficient: unit tests exercise component logic and
// text content, not SVG rendering output.
function SvgComponent(props) {
    return null
}

const knownExports = {
    Svg: SvgComponent,
    G: SvgComponent,
    Path: SvgComponent,
    Circle: SvgComponent,
    Ellipse: SvgComponent,
    Line: SvgComponent,
    Rect: SvgComponent,
    Polygon: SvgComponent,
    Polyline: SvgComponent,
    Defs: SvgComponent,
    ClipPath: SvgComponent,
    Pattern: SvgComponent,
    Mask: SvgComponent,
    LinearGradient: SvgComponent,
    RadialGradient: SvgComponent,
    Stop: SvgComponent,
    Symbol: SvgComponent,
    Use: SvgComponent,
    Image: SvgComponent,
    ForeignObject: SvgComponent,
    Marker: SvgComponent,
    TSpan: SvgComponent,
    TextPath: SvgComponent,
    Text: SvgComponent,
}

// Any other named export (SvgUri, SvgXml, SvgCss, fetchText, camelCase, parse,
// the RNSVG* fabric component refs, …) resolves to the same no-op — nothing
// in the unit-test import chain calls these directly, they're only re-exported.
const handler = {
    get(target, prop) {
        if (prop === '__esModule') return true
        if (prop === 'default') return SvgComponent
        if (prop in target) return target[prop]
        return SvgComponent
    },
}

module.exports = new Proxy(knownExports, handler)
