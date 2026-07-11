const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'] as const

export function formatBytes(bytes: number): string {
    if (bytes === 0) return '—'
    // A non-finite (NaN/Infinity) or negative size is never a real file size;
    // treat it as nothing rather than emitting "NaN undefined".
    if (!Number.isFinite(bytes) || bytes < 0) return '0 B'
    // Bytes below 1 KiB (incl. fractional counts < 1) belong in the 'B' unit;
    // log-based index math would otherwise produce a negative exponent.
    if (bytes < 1024) return `${Math.round(bytes)} B`
    // Clamp the exponent to the last available unit so a petabyte-plus value
    // still renders in 'PB' instead of indexing past the array into undefined.
    const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), BYTE_UNITS.length - 1)
    const value = bytes / 1024 ** exponent
    return `${value.toFixed(1)} ${BYTE_UNITS[exponent]}`
}

export function formatDate(isoDate: string): string {
    // Render nothing for an absent date rather than the literal "Invalid Date".
    // The only source of an empty value is a caller that has no date to pass
    // (e.g. a drive search row before its record is loaded); our API otherwise
    // always emits well-formed dates.
    if (!isoDate) return ''
    // Use the device locale (undefined) to match formatRelativeDate — an
    // absolute date shouldn't render as en-US on a non-US device.
    return new Date(isoDate).toLocaleDateString(undefined, {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
    })
}

export function formatRelativeDate(isoDate: string): string {
    if (!isoDate) return ''

    const date = new Date(isoDate)
    const now = new Date()

    if (Number.isNaN(date.getTime())) return isoDate

    const isToday =
        date.getFullYear() === now.getFullYear() &&
        date.getMonth() === now.getMonth() &&
        date.getDate() === now.getDate()

    if (isToday) {
        return date.toLocaleTimeString(undefined, {
            hour: 'numeric',
            minute: '2-digit',
        })
    }

    const yesterday = new Date(now)
    yesterday.setDate(yesterday.getDate() - 1)
    const isYesterday =
        date.getFullYear() === yesterday.getFullYear() &&
        date.getMonth() === yesterday.getMonth() &&
        date.getDate() === yesterday.getDate()

    if (isYesterday) return 'Yesterday'

    if (date.getFullYear() === now.getFullYear()) {
        return date.toLocaleDateString(undefined, {
            month: 'short',
            day: 'numeric',
        })
    }

    return date.toLocaleDateString(undefined, {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
    })
}
