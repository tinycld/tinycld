// Test-only stand-in for expo-file-system. The real package is a native module
// that cannot load in Node. This one keeps "files on disk" in memory and
// records uploads, so the native upload helpers (core/lib/upload-file.native.ts,
// core/lib/native-upload-body.native.ts, core/file-viewer/xhr-form-data.native.ts)
// can run under vitest. Tests reach the recorder by importing this file directly;
// code under test imports `expo-file-system`, which vitest aliases here.

const disk = new Map<string, Uint8Array<ArrayBuffer>>()

interface RecordedUpload {
    fileUri: string
    fileName: string
    url: string
    options: UploadOptions
}

const uploads: RecordedUpload[] = []
let nextResult: UploadResult = { status: 200, body: '{}', headers: {} }

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
    reset() {
        disk.clear()
        uploads.length = 0
        nextResult = { status: 200, body: '{}', headers: {} }
    },
}

export enum UploadType {
    BINARY_CONTENT = 0,
    MULTIPART = 1,
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
    async copy(destination: File) {
        disk.set(destination.uri, disk.get(this.uri) ?? new Uint8Array(0))
    }
    delete() {
        disk.delete(this.uri)
    }
    async upload(url: string, options: UploadOptions = {}): Promise<UploadResult> {
        if (!disk.has(this.uri)) throw new Error(`no such file: ${this.uri}`)
        uploads.push({ fileUri: this.uri, fileName: this.name, url, options })
        return nextResult
    }
}

export const Paths = {
    cache: new Directory('file:///cache'),
}
