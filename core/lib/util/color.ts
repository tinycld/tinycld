// Deterministic HSL color string from an id — same id always returns the same color.
export function colorForUser(id: string): string {
    let h = 0
    for (let i = 0; i < id.length; i++) {
        h = (h * 31 + id.charCodeAt(i)) >>> 0
    }
    const hue = h % 360
    return `hsl(${hue}, 70%, 45%)`
}
