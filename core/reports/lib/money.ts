import { log } from '@tinycld/core/lib/logger'

// Amounts are integer cents. PocketBase stores numbers as float64, exact only up
// to 2^53-1, so a larger amount cannot be shown truthfully.
export const MAX_STORED_AMOUNT = Number.MAX_SAFE_INTEGER

// The epsilon nudge makes 1000.4999999999999 (10.005 * 100) round as a person
// reading the decimal expects.
export function roundHalfAwayFromZero(value: number): number {
    return Math.sign(value) * Math.round(Math.abs(value) * (1 + Number.EPSILON))
}

export function groupDigits(magnitude: number): string {
    return String(magnitude).replace(/\B(?=(\d{3})+$)/g, ',')
}

const warned = new Set<string>()

function warnOnce(key: string, message: string, extra: Record<string, unknown>) {
    if (warned.has(key)) return
    warned.add(key)
    log.warn('reports.money', message, extra)
}

// cents / 100 as a float loses the last cent near 2^53, so whole cents are
// written from their digits; Intl supplies only the layout (symbol, sign,
// separators) through formatToParts of a sample value.
function formatCents(format: Intl.NumberFormat, cents: number): string {
    if (!Number.isSafeInteger(cents)) return format.format(cents / 100)
    const magnitude = Math.abs(cents)
    const whole = groupDigits(Math.floor(magnitude / 100))
    const fraction = String(magnitude % 100).padStart(2, '0')
    return format
        .formatToParts(cents < 0 ? -1 : 1)
        .map(part => {
            if (part.type === 'integer') return whole
            if (part.type === 'fraction') return fraction
            return part.value
        })
        .join('')
}

export function formatMoney(cents: number, currency: string): string {
    if (!Number.isFinite(cents)) {
        warnOnce('non-finite', 'non-finite amount', { cents })
        return '—'
    }
    // A value that rounds to zero would otherwise read "-$0.00".
    const amount = Math.abs(cents) < 0.5 ? 0 : cents
    const digits = { minimumFractionDigits: 2, maximumFractionDigits: 2 }
    if (currency !== '') {
        try {
            const currencyFormat = new Intl.NumberFormat('en-US', {
                style: 'currency',
                currency,
                ...digits,
            })
            return formatCents(currencyFormat, amount)
        } catch (err) {
            warnOnce(`currency:${currency}`, 'invalid currency code', { currency, err })
        }
    }
    const plain = formatCents(new Intl.NumberFormat('en-US', digits), amount)
    return currency === '' ? plain : `${plain} ${currency}`
}
