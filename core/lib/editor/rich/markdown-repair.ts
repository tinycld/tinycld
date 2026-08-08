/**
 * Repairs two content-corrupting defects in `@tiptap/markdown@3.29.2`'s
 * serializer. Both were found by round-tripping real content against the Go
 * serializer's corpus (`tinycld/core/server/markdown/testdata/corpus/`).
 *
 * 1. **A code span containing a backtick gets a single-backtick fence**
 *    (`` `a ` b` ``). That re-parses as a broken span: everything after the
 *    inner backtick escapes it. CommonMark requires a fence longer than the
 *    longest run inside, padded with a space on each side.
 *
 * 2. **Table cell pipes are not escaped**, so a cell reading `x | y` ends the
 *    cell early and shifts every column after it. Re-reading such a table
 *    silently loses a cell.
 *
 * ## Why this is a post-pass and not an extension override
 *
 * The obvious fix — `Code.extend({ renderMarkdown })` — cannot work. The
 * library serializes a MARK by rendering it around a sentinel placeholder and
 * slicing the result into an opening and a closing delimiter
 * (`MarkdownManager.getMarkOpening` / `getMarkClosing`). The handler is never
 * shown the real text, so a fence whose length depends on the content is not
 * expressible through that API. Registering a second extension with the same
 * name does not help either: tiptap warns about the duplicate and keeps the
 * first registration.
 *
 * So the repair runs on the emitted string, where the content is finally
 * visible. Both passes are careful to be idempotent — running them twice must
 * not add another layer of fencing or escaping — because the output is compared
 * against a stored baseline to decide whether a document changed.
 */

/** Longest run of backticks in a string. */
function longestBacktickRun(text: string): number {
    let longest = 0
    for (const run of text.match(/`+/g) ?? []) {
        longest = Math.max(longest, run.length)
    }
    return longest
}

/**
 * Re-fence inline code spans whose content contains a backtick.
 *
 * Scans for single-backtick-delimited spans; a span already carrying a longer
 * fence is well-formed and left alone, which is what makes this idempotent.
 */
function repairCodeSpans(markdown: string): string {
    let out = ''
    let i = 0
    while (i < markdown.length) {
        const char = markdown[i]
        if (char !== '`') {
            // A fenced code block is not our business — skip to its end so a
            // stray backtick inside it is never treated as a span.
            out += char
            i++
            continue
        }

        // Measure the opening fence.
        let fenceLen = 0
        while (markdown[i + fenceLen] === '`') fenceLen++
        const fence = '`'.repeat(fenceLen)

        // A run of three or more at the start of a line opens a block.
        if (fenceLen >= 3 && (i === 0 || markdown[i - 1] === '\n')) {
            const close = markdown.indexOf(`\n${fence}`, i + fenceLen)
            const end = close === -1 ? markdown.length : close + fenceLen + 1
            out += markdown.slice(i, end)
            i = end
            continue
        }

        const closeIndex = markdown.indexOf(fence, i + fenceLen)
        if (closeIndex === -1) {
            out += fence
            i += fenceLen
            continue
        }

        if (fenceLen === 1) {
            // A single-backtick fence is the broken shape. Its content is
            // ambiguous by construction — `a ` b` could be read as the span
            // "a " followed by prose, or as the span "a ` b" the author meant.
            // The serializer only ever produces this by under-fencing a span
            // that held a backtick, so prefer the author's reading and take the
            // LAST backtick on the line as the close.
            const lineEnd = markdown.indexOf('\n', i)
            const searchLimit = lineEnd === -1 ? markdown.length : lineEnd
            const lastTick = markdown.lastIndexOf('`', searchLimit - 1)
            if (lastTick > closeIndex) {
                const inner = markdown.slice(i + 1, lastTick)
                const widened = '`'.repeat(longestBacktickRun(inner) + 1)
                out += `${widened} ${inner} ${widened}`
                i = lastTick + 1
                continue
            }
        }

        const inner = markdown.slice(i + fenceLen, closeIndex)
        out += fence + inner + fence
        i = closeIndex + fenceLen
    }
    return out
}

/** A line that looks like a GFM table row. */
function isTableRow(line: string): boolean {
    return line.trimStart().startsWith('|') && line.trimEnd().endsWith('|')
}

/** A table's delimiter row (`| --- | --- |`). */
function isDelimiterRow(line: string): boolean {
    return /^\s*\|(?:\s*:?-+:?\s*\|)+\s*$/.test(line)
}

/**
 * Table-pipe damage is detectable but NOT repairable from the emitted string.
 *
 * When a cell's text contains an unescaped pipe, the row gains a column and
 * there is no way to tell which pipe was the author's: `| pipe | inside |
 * plain |` in a two-column table could be `pipe | inside` + `plain`, or `pipe`
 * + `inside | plain`. Guessing corrupts the table in a new way half the time,
 * which is worse than the original bug because it looks deliberate.
 *
 * So this reports rather than rewrites. The real fix is upstream, and the
 * architectural protection is that persisted markdown is serialized by the Go
 * writer (`tinycld/core/server/markdown`), which escapes cell pipes correctly.
 * Editor output is used for content a person just typed, where the damage is
 * visible immediately and can be corrected by hand.
 */
export function findDamagedTableRows(markdown: string): string[] {
    const lines = markdown.split('\n')
    const damaged: string[] = []

    for (let i = 0; i < lines.length; i++) {
        if (!isDelimiterRow(lines[i])) continue
        const columns = splitRow(lines[i]).length
        if (columns === 0) continue
        for (const rowIndex of tableRowIndexes(lines, i)) {
            if (splitRow(lines[rowIndex]).length > columns) {
                damaged.push(lines[rowIndex])
            }
        }
    }
    return damaged
}

/** Row indexes belonging to the table whose delimiter row sits at delimiter. */
function tableRowIndexes(lines: string[], delimiter: number): number[] {
    const indexes: number[] = []
    if (delimiter > 0 && isTableRow(lines[delimiter - 1])) indexes.push(delimiter - 1)
    for (let i = delimiter + 1; i < lines.length && isTableRow(lines[i]); i++) {
        indexes.push(i)
    }
    return indexes
}

/** Split a table row into cells on unescaped pipes. */
function splitRow(line: string): string[] {
    const trimmed = line.trim().replace(/^\|/, '').replace(/\|$/, '')
    const cells: string[] = []
    let current = ''
    for (let i = 0; i < trimmed.length; i++) {
        const char = trimmed[i]
        if (char === '\\' && trimmed[i + 1] === '|') {
            current += '\\|'
            i++
            continue
        }
        if (char === '|') {
            cells.push(current)
            current = ''
            continue
        }
        current += char
    }
    cells.push(current)
    return cells
}

/**
 * Repair a markdown string emitted by `editor.getMarkdown()`.
 *
 * Always run this before persisting or comparing editor output. Idempotent:
 * repairing an already-repaired string returns it unchanged.
 *
 * Fixes code-span fencing. Table-pipe damage is detected but deliberately not
 * rewritten — see findDamagedTableRows for why guessing is worse than leaving
 * it.
 */
export function repairMarkdown(markdown: string): string {
    return repairCodeSpans(markdown)
}
