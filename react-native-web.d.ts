// react-native-web accepts a `dataSet` prop on View/Pressable and renders each
// key as a `data-*` attribute, but @types/react-native doesn't model it.
//
// Drive carries the same augment locally (drive/tinycld/drive/types/
// react-native-web.d.ts) so its own typecheck passes, but that ambient file is
// NOT part of the app-shell tsc program — the app shell reaches drive only
// through the generated route re-exports (app/a/[orgSlug]/drive/index.tsx →
// `@tinycld/drive/screens/index`), which pull in drive's source for type
// checking without its sibling `.d.ts` files. So the augment must also live here
// for the app-shell typecheck (and any other member that tags Pressables with
// `dataSet`). `PressableProps extends Omit<ViewProps, …>`, so augmenting
// ViewProps covers Pressable too.
import 'react-native'

declare module 'react-native' {
    interface ViewProps {
        /** react-native-web only: each entry renders as a `data-<kebab-key>`
         *  DOM attribute. No-op on native. */
        dataSet?: Record<string, string | number | undefined>
    }
}
