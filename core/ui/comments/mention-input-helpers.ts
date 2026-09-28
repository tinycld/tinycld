// Pure helpers for MentionInput. Kept out of the component module so
// tests can exercise the logic without pulling react-native (which
// uses Flow syntax that vitest can't parse and only has a small
// shim in tests/unit-setup.ts).

export interface MentionTrigger {
    // Inclusive index of the `@` that opened the autocomplete.
    atIndex: number
    // Caret index, == end of the query being typed.
    caretIndex: number
    // Substring between `@` and caret. May be empty just after typing `@`.
    query: string
}

// Returns the active @-trigger info, or null when the caret isn't
// inside a fresh `@<query>` token. The trigger only fires when `@` is
// at the start of the body or follows whitespace — otherwise typing
// "user@example.com" would auto-open the popover on the email host.
// Bails when the query already contains whitespace — by then the user
// has typed past the mention.
export function detectTrigger(value: string, caretIndex: number): MentionTrigger | null {
    if (caretIndex <= 0 || caretIndex > value.length) return null
    let i = caretIndex - 1
    while (i >= 0) {
        const ch = value[i]
        if (ch === '@') {
            const prev = i > 0 ? value[i - 1] : null
            if (prev !== null && !/\s/.test(prev)) return null
            const query = value.slice(i + 1, caretIndex)
            if (/\s/.test(query)) return null
            return { atIndex: i, caretIndex, query }
        }
        if (/\s/.test(ch)) return null
        i -= 1
    }
    return null
}

// Renders a comment body string with `[[@id]]` tokens replaced by
// `@<displayName>`. Falls back to `@<id>` when the lookup misses
// (the user was removed from the org since the comment was posted).
export function renderMentionsToText(body: string, nameByUserId: Map<string, string>): string {
    return body.replace(/\[\[@([A-Za-z0-9_-]+)\]\]/g, (_, id: string) => {
        const name = nameByUserId.get(id)
        return name ? `@${name}` : `@${id}`
    })
}

// The text and selection the input held before an edit.
export interface MentionSelectionSnapshot {
    text: string
    // Both ends of the selection. Equal when the caret is collapsed.
    start: number
    end: number
}

// Where the caret lands after the input's text changes.
//
// An edit replaces the selected range with whatever was inserted, so the
// caret ends up at `start + insertedLength`. The inserted length is what
// the length delta leaves once the removed range is accounted for:
//
//     next.length - prev.text.length = inserted - (end - start)
//
// so `inserted = next.length - prev.text.length + (end - start)`, and the
// caret is `start + inserted`.
//
// The collapsed case (`start === end`) reduces to the plain
// "caret moves by the length delta" rule, which is why it went unnoticed.
// A non-collapsed one does not: selecting five characters and pressing
// Delete yields inserted = 0 and a caret at `start`, whereas the delta
// alone would put it five characters EARLIER than the selection began —
// far enough back to detect an unrelated `@…` and splice on its offsets.
//
// The result is clamped into the new text because a platform can deliver
// a stale selection alongside a programmatic value change.
export function caretAfterEdit(prev: MentionSelectionSnapshot, next: string): number {
    const removed = Math.max(0, prev.end - prev.start)
    const inserted = next.length - prev.text.length + removed
    return Math.min(Math.max(prev.start + inserted, 0), next.length)
}
