// react-native-web writes a View's dataSet as data-* attributes (keys hyphenated:
// fitWidth → data-fit-width); React Native's types do not declare it. Native
// ignores it. Used by the report table's e2e marker.
import 'react-native'

declare module 'react-native' {
    interface ViewProps {
        dataSet?: Record<string, string>
    }
}
