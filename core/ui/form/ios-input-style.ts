import { Platform, type TextStyle } from 'react-native'

// iOS renders a single-line TextInput's glyph at the TOP of its line box rather
// than centering it the way Android and the web do. Tailwind/Uniwind's text-*
// utilities ship a line-height taller than the font (e.g. text-base = 16px font
// / 24px line-height), so on iOS the text visibly hugs the top of the padded
// box. Pinning lineHeight close to the font size collapses that extra leading so
// the glyph centers within the input's vertical padding. Android/web already
// center, and a single line is unaffected by the smaller line-height there — so
// this is scoped to iOS to leave those platforms byte-identical.
//
// Pass the input's font size (px); lineHeight is set a hair above it to leave
// room for descenders. Returns undefined off iOS so callers can spread it
// unconditionally.
export function iosInputCenteringStyle(fontSize: number): TextStyle | undefined {
    if (Platform.OS !== 'ios') return undefined
    return { lineHeight: fontSize + 4 }
}
