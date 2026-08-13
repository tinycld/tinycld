// Ambient declaration for lucide per-icon deep imports.
//
// We import individual icons as `lucide-react-native/icons/<kebab>` to avoid
// pulling the whole ~1700-icon barrel into the bundle (Metro doesn't tree-shake;
// see metro.config.cjs, which remaps this specifier to the real per-icon module).
// lucide's `exports` map only publishes `.` and `./icons`, so TypeScript can't
// resolve the bare deep path on its own — declare it here. Each icon module has
// a default export of the standard lucide component type.
//
// Lives in core (not just the app shell) and is wired into every package's
// `types` via tsconfig.package-base.json, because any sibling that reaches
// `@tinycld/app-generated/*` transitively reaches the app shell's generated
// `lib/generated/package-icons.ts`, which uses this same deep-import form.
declare module 'lucide-react-native/icons/*' {
    import type { LucideIcon } from 'lucide-react-native'

    const icon: LucideIcon
    export default icon
}
