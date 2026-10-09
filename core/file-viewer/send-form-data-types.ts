export interface SendFormDataParams {
    url: string
    formData: FormData
    authToken: string
    method: string
    onProgress?: (loaded: number, total: number) => void
    signal?: AbortSignal
}

/** One upload attempt's outcome. An HTTP error status is an outcome, not a rejection. */
export interface SendFormDataResult {
    status: number
    body: unknown
    retryAfter: string | null
}
