export interface PowChallenge {
    algorithm: 'SHA-256'
    challenge: string
    maxnumber: number
    salt: string
    signature: string
}

async function subtleDigest(s: string): Promise<string> {
    const bytes = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(s))
    return Array.from(new Uint8Array(bytes), b => b.toString(16).padStart(2, '0')).join('')
}

const nextTick = () => new Promise<void>(resolve => setTimeout(resolve, 0))

/**
 * Finds n with sha256(salt + n) == challenge. Hashes a batch in parallel and
 * yields between batches so the page stays responsive; Metro has no Web Worker
 * set up, so it runs on the main thread.
 */
export async function solveChallenge(
    c: PowChallenge,
    opts: { batch?: number; digest?: (s: string) => Promise<string> } = {}
): Promise<string> {
    const batch = opts.batch ?? 500
    const digest = opts.digest ?? subtleDigest
    for (let start = 0; start <= c.maxnumber; start += batch) {
        const end = Math.min(start + batch - 1, c.maxnumber)
        const numbers = Array.from({ length: end - start + 1 }, (_, i) => start + i)
        const hashes = await Promise.all(numbers.map(n => digest(`${c.salt}${n}`)))
        const hit = hashes.indexOf(c.challenge)
        if (hit !== -1) {
            const solution = {
                algorithm: c.algorithm,
                challenge: c.challenge,
                number: numbers[hit],
                salt: c.salt,
                signature: c.signature,
            }
            return btoa(JSON.stringify(solution))
        }
        await nextTick()
    }
    throw new Error('Proof of work: no solution in range')
}
