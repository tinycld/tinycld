// Test-only stand-in for expo-file-system. The real package is a native module
// that cannot load in Node. This one keeps "files on disk" in memory and
// records uploads, so the native upload helpers (core/lib/upload-file.native.ts,
// core/lib/native-upload.native.ts and their callers) can run under vitest.
// Tests reach the recorder by importing this file directly; code under test
// imports `expo-file-system`, which vitest aliases here.

const disk = new Map<string, Uint8Array<ArrayBuffer>>()

interface RecordedUpload {
    fileUri: string
    fileName: string
    /** The file's bytes when the upload started — for a BINARY_CONTENT upload, the whole body. */
    bytes: Uint8Array<ArrayBuffer>
    url: string
    options: UploadOptions
}

const uploads: RecordedUpload[] = []
let nextResult: UploadResult = { status: 200, body: '{}', headers: {} }
let progressSteps = 2

export const expoFileSystemStub = {
    writeFile(uri: string, bytes: Uint8Array<ArrayBuffer>) {
        disk.set(uri, bytes)
    },
    hasFile(uri: string) {
        return disk.has(uri)
    },
    uploads,
    respondWith(result: UploadResult) {
        nextResult = result
    },
    /** How many progress events an upload reports before it completes. */
    reportProgressSteps(steps: number) {
        progressSteps = steps
    },
    reset() {
        disk.clear()
        uploads.length = 0
        nextResult = { status: 200, body: '{}', headers: {} }
        progressSteps = 2
    },
}

export enum UploadType {
    BINARY_CONTENT = 0,
    MULTIPART = 1,
}

export enum FileMode {
    ReadWrite = 'rw',
    ReadOnly = 'r',
}

export interface UploadResult {
    body: string
    status: number
    headers: Record<string, string>
}

export interface UploadOptions {
    httpMethod?: 'POST' | 'PUT' | 'PATCH'
    uploadType?: UploadType
    headers?: Record<string, string>
    fieldName?: string
    mimeType?: string
    parameters?: Record<string, string>
    onProgress?: (data: { bytesSent: number; totalBytes: number }) => void
    signal?: AbortSignal
    sessionType?: 'background' | 'foreground'
}

const MIME_BY_EXTENSION: Record<string, string> = {
    pdf: 'application/pdf',
    jpg: 'image/jpeg',
    png: 'image/png',
    txt: 'text/plain',
}

function joinUri(parts: (string | File | Directory)[]): string {
    return parts
        .map(part => (typeof part === 'string' ? part : part.uri))
        .reduce((joined, part) => `${joined.replace(/\/$/, '')}/${part.replace(/^\//, '')}`)
}

function baseName(uri: string): string {
    return uri.split('/').pop() ?? ''
}

export class Directory {
    readonly uri: string
    constructor(...parts: (string | File | Directory)[]) {
        this.uri = joinUri(parts)
    }
    get exists() {
        return [...disk.keys()].some(uri => uri.startsWith(`${this.uri}/`))
    }
    create(_options?: { intermediates?: boolean; idempotent?: boolean }) {}
    delete() {
        for (const uri of [...disk.keys()]) {
            if (uri.startsWith(`${this.uri}/`)) disk.delete(uri)
        }
    }
}

class FileHandle {
    offset: number | null = 0
    constructor(private readonly uri: string) {}
    get size(): number | null {
        return disk.get(this.uri)?.byteLength ?? 0
    }
    readBytes(length: number): Uint8Array<ArrayBuffer> {
        const bytes = disk.get(this.uri) ?? new Uint8Array(0)
        const start = this.offset ?? 0
        const chunk = bytes.slice(start, start + length)
        this.offset = start + chunk.byteLength
        return chunk
    }
    writeBytes(bytes: Uint8Array) {
        const existing = disk.get(this.uri) ?? new Uint8Array(0)
        const next = new Uint8Array(existing.byteLength + bytes.byteLength)
        next.set(existing)
        next.set(bytes, existing.byteLength)
        disk.set(this.uri, next)
        this.offset = next.byteLength
    }
    close() {
        this.offset = null
    }
}

class UploadTask {
    constructor(
        private readonly file: File,
        private readonly url: string,
        private readonly options: UploadOptions
    ) {}
    async uploadAsync(): Promise<UploadResult> {
        const { signal, onProgress } = this.options
        if (signal?.aborted) throw new Error('UploadCancelledException')
        const bytes = disk.get(this.file.uri)
        if (!bytes) throw new Error(`no such file: ${this.file.uri}`)
        uploads.push({
            fileUri: this.file.uri,
            fileName: this.file.name,
            bytes,
            url: this.url,
            options: this.options,
        })
        for (let step = 1; step <= progressSteps; step++) {
            await Promise.resolve()
            if (signal?.aborted) throw new Error('UploadCancelledException')
            onProgress?.({
                bytesSent: Math.round((bytes.byteLength * step) / progressSteps),
                totalBytes: bytes.byteLength,
            })
        }
        return nextResult
    }
}

// Extends Node's File so the stub is a real Blob that FormData keeps as is.
export class File extends globalThis.File {
    readonly uri: string
    constructor(...parts: (string | File | Directory)[]) {
        const uri = joinUri(parts)
        const name = baseName(uri)
        const extension = name.split('.').pop() ?? ''
        super([disk.get(uri) ?? new Uint8Array(0)], name, {
            type: MIME_BY_EXTENSION[extension] ?? '',
        })
        this.uri = uri
    }
    get exists() {
        return disk.has(this.uri)
    }
    create() {
        disk.set(this.uri, new Uint8Array(0))
    }
    open(_mode?: FileMode) {
        return new FileHandle(this.uri)
    }
    async copy(destination: File) {
        disk.set(destination.uri, disk.get(this.uri) ?? new Uint8Array(0))
    }
    delete() {
        disk.delete(this.uri)
    }
    createUploadTask(url: string, options: UploadOptions = {}) {
        return new UploadTask(this, url, options)
    }
}

export const Paths = {
    cache: new Directory('file:///cache'),
}
