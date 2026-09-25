import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'

export interface SetupErrorBody {
    error?: string
    reason?: string
}

/** A refused or failed /api/setup call. `status` and `body` are null when the server did not answer. */
export class SetupRequestError extends Error {
    constructor(
        readonly status: number | null,
        readonly body: SetupErrorBody | null
    ) {
        super(body?.error ?? 'The server did not answer.')
    }
}

async function readBody(res: Response): Promise<SetupErrorBody | null> {
    try {
        return (await res.json()) as SetupErrorBody
    } catch {
        return null
    }
}

// These endpoints run before any account exists, so they are plain fetches:
// there is no PocketBase auth or record to go through.
export async function postSetup<T>(path: 'verify' | 'init', payload: object): Promise<T> {
    let res: Response
    try {
        res = await fetch(`${PB_SERVER_ADDR}/api/setup/${path}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
        })
    } catch {
        throw new SetupRequestError(null, null)
    }
    if (!res.ok) throw new SetupRequestError(res.status, await readBody(res))
    return (await res.json()) as T
}

export function formatSetupCode(code: string): string {
    const chars = code.replace(/[^A-Z0-9]/gi, '').toUpperCase()
    if (chars.length !== 8) return ''
    return `${chars.slice(0, 4)}-${chars.slice(4)}`
}
