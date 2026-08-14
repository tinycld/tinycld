/**
 * Escape text that is about to be spliced into markdown SOURCE.
 *
 * The case this exists for: a mention renders as `@<display name>` inserted
 * into a comment body before the markdown parser runs. Display names are
 * user-controlled — `users.updateRule` lets any user set their own `name` —
 * so an unescaped name is an injection into someone else's document. A name of
 * `[click](javascript:alert(1))` becomes a real link node, and
 * `![x](https://evil.test/p.png)` becomes an image the reader's client fetches
 * with no interaction at all.
 *
 * Core's editor path already escapes the same value on the way OUT
 * (escapeHtmlAttr in lib/editor/rich/mention-node.ts, "the name is
 * user-controlled, so it must not be able to close the attribute"). This is
 * that treatment for the read path, and it lives here rather than in any one
 * package so a second package rendering mention tokens inherits it.
 *
 * Escapes the CommonMark punctuation run wholesale rather than trying to spot
 * dangerous constructs. A backslash escape is valid before any ASCII
 * punctuation, and enumerating "just the harmful ones" is how the next
 * construct gets missed — `[` and `(` alone would still leave `!` free to pair
 * with an already-escaped bracket in a later edit.
 */
const MARKDOWN_PUNCTUATION = /[\\`*_{}[\]()#+\-.!<>|~]/g

export function escapeMarkdown(value: string): string {
    return value.replace(MARKDOWN_PUNCTUATION, '\\$&')
}
