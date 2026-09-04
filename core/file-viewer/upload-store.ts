import { create } from '../lib/store'

export type UploadStatus = 'pending' | 'uploading' | 'done' | 'error'

export interface UploadingFile {
    id: string
    name: string
    /**
     * Whatever the surface groups uploads by — drive would pass a folder id,
     * boards passes the card id. The store never interprets it; it exists so
     * one screen's in-flight uploads don't render on another's.
     */
    scopeId: string
    size: number
    loaded: number
    status: UploadStatus
    errorMessage?: string
    /**
     * Local URI for an image being uploaded, so the surface can show the
     * picture itself while the bytes are still in flight. There is no record
     * and no server URL yet, so this is the only thing there is to draw.
     */
    previewUri?: string
}

interface UploadStoreState {
    uploadingFiles: UploadingFile[]
    add: (entries: UploadingFile[]) => void
    update: (id: string, patch: Partial<UploadingFile>) => void
    remove: (id: string) => void
    clearDoneById: (id: string) => void
}

/**
 * In-flight uploads, shared across the components that display them.
 *
 * A store rather than local state because the surface that starts an upload
 * is rarely the only one that reports it — a strip, a toolbar and a row can
 * all need the same list. Read it with a selector narrowed to one scope
 * (`useUploadsForScope`) so an unrelated upload elsewhere doesn't re-render.
 */
export const useUploadStore = create<UploadStoreState>(set => ({
    uploadingFiles: [],
    add: entries =>
        set(s => ({
            uploadingFiles: [...s.uploadingFiles, ...entries],
        })),
    update: (id, patch) =>
        set(s => ({
            uploadingFiles: s.uploadingFiles.map(f => (f.id === id ? { ...f, ...patch } : f)),
        })),
    remove: id =>
        set(s => ({
            uploadingFiles: s.uploadingFiles.filter(f => f.id !== id),
        })),
    clearDoneById: id =>
        set(s => ({
            uploadingFiles: s.uploadingFiles.filter(f => !(f.id === id && f.status === 'done')),
        })),
}))

/** Fraction in [0,1]. A zero-byte or not-yet-sized upload reads as 0. */
export function uploadProgress(file: UploadingFile): number {
    if (file.size <= 0) return 0
    return Math.min(1, file.loaded / file.size)
}

const EMPTY: UploadingFile[] = []

/**
 * The in-flight uploads for one scope.
 *
 * Filtering inside the selector would allocate a new array on every store
 * write and defeat zustand's identity check, so this subscribes to the raw
 * list and narrows outside the selector — and returns a shared constant when
 * there is nothing in flight, which is almost always.
 */
export function useUploadsForScope(scopeId: string): UploadingFile[] {
    const all = useUploadStore(s => s.uploadingFiles)
    if (all.length === 0) return EMPTY
    const mine = all.filter(f => f.scopeId === scopeId)
    return mine.length === 0 ? EMPTY : mine
}
