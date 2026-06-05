// A small Unicode box printer for terminal output, mirroring the look of the
// Go server's printBoxed (core/server/coreserver/setup_bootstrap.go). Used by
// the seed / db:reset scripts to surface login credentials and setup links so a
// contributor doesn't have to read source to find them.
//
// Box width is sized to the widest line. Lines are left-aligned with a single
// space of padding inside the border.

export function printBox(lines: string[]): void {
    const width = Math.max(...lines.map(line => line.length)) + 2
    const horizontal = '─'.repeat(width)

    const pad = (s: string) => `│ ${s}${' '.repeat(width - s.length - 2)} │`

    process.stdout.write(`\n┌${horizontal}┐\n`)
    for (const line of lines) {
        process.stdout.write(`${pad(line)}\n`)
    }
    process.stdout.write(`└${horizontal}┘\n\n`)
}
