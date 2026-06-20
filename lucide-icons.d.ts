// Ambient declaration for lucide per-icon deep imports.
//
// We import individual icons as `lucide-react-native/icons/<kebab>` to avoid
// pulling the whole ~1700-icon barrel into the bundle (Metro doesn't tree-shake;
// see metro.config.cjs, which remaps this specifier to the real per-icon module).
// lucide's `exports` map only publishes `.` and `./icons`, so TypeScript can't
// resolve the bare deep path on its own — declare it here. Each icon module has
// a default export of the standard lucide component type.
declare module 'lucide-react-native/icons/*' {
    import type { LucideIcon } from 'lucide-react-native'

    const icon: LucideIcon
    export default icon
}
