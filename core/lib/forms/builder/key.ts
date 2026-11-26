const MAX_KEY = 40

/** A field key from its label: lowercase ASCII, underscores, starts with a letter. Runs only when a field is created. */
export function keyFromLabel(label: string): string {
    const ascii = label
        .normalize('NFKD')
        .replace(/[̀-ͯ]/g, '')
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, '_')
        .replace(/^_+|_+$/g, '')
    if (ascii === '') return 'field'
    const prefixed = /^[a-z]/.test(ascii) ? ascii : `f_${ascii}`
    return prefixed.slice(0, MAX_KEY).replace(/_+$/, '')
}
