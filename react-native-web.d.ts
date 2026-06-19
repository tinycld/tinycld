// react-native-web accepts a `dataSet` prop on View/Pressable and renders each
// key as a `data-*` attribute, but @types/react-native doesn't model it.
//
// This augment lives in the APP SHELL (not only in the package that uses
// dataSet) because the app's typecheck compiles every package's screens through
// the generated route re-exports, and that compilation does NOT include a
// sibling package's own `types/*.d.ts`. drive (and any other package) tags rows
// with `dataSet={{ ... }}` for its web drag-to-select hit-test; without this the
// app-shell typecheck fails with "Property 'dataSet' does not exist on
// PressableProps" even though the package typechecks clean standalone.
//
// PressableProps extends Omit<ViewProps, …> (dataSet is not omitted), so
// augmenting ViewProps covers both View and Pressable.
import 'react-native'

declare module 'react-native' {
    interface ViewProps {
        /** react-native-web only: each entry renders as a `data-<kebab-key>`
         *  DOM attribute. No-op on native. */
        dataSet?: Record<string, string | number | undefined>
    }
}
