import {
    type UploadingFile,
    uploadProgress,
    useUploadStore,
} from '@tinycld/core/file-viewer/upload-store'
import { beforeEach, describe, expect, it } from 'vitest'

function entry(patch: Partial<UploadingFile> = {}): UploadingFile {
    return {
        id: 'u1',
        name: 'photo.png',
        scopeId: 'card1',
        size: 1000,
        loaded: 0,
        status: 'pending',
        ...patch,
    }
}

beforeEach(() => {
    useUploadStore.setState({ uploadingFiles: [] })
})

describe('useUploadStore', () => {
    it('appends rather than replacing, so two scopes can upload at once', () => {
        const { add } = useUploadStore.getState()
        add([entry({ id: 'a', scopeId: 'card1' })])
        add([entry({ id: 'b', scopeId: 'card2' })])

        expect(useUploadStore.getState().uploadingFiles.map(f => f.id)).toEqual(['a', 'b'])
    })

    it('patches one row and leaves its siblings alone', () => {
        const { add, update } = useUploadStore.getState()
        add([entry({ id: 'a' }), entry({ id: 'b' })])

        update('a', { loaded: 500, status: 'uploading' })

        const [a, b] = useUploadStore.getState().uploadingFiles
        expect(a).toMatchObject({ id: 'a', loaded: 500, status: 'uploading' })
        expect(b).toMatchObject({ id: 'b', loaded: 0, status: 'pending' })
    })

    it('removes a row outright', () => {
        const { add, remove } = useUploadStore.getState()
        add([entry({ id: 'a' }), entry({ id: 'b' })])

        remove('a')

        expect(useUploadStore.getState().uploadingFiles.map(f => f.id)).toEqual(['b'])
    })

    it('clears a finished row by id', () => {
        const { add, clearDoneById } = useUploadStore.getState()
        add([entry({ id: 'a', status: 'done' })])

        clearDoneById('a')

        expect(useUploadStore.getState().uploadingFiles).toEqual([])
    })

    it('leaves a still-running row alone when asked to clear it', () => {
        const { add, clearDoneById } = useUploadStore.getState()
        add([entry({ id: 'a', status: 'uploading' })])

        // The auto-clear timer fires on a delay, and the row may have been
        // retried in the meantime — clearing it then would hide a live upload.
        clearDoneById('a')

        expect(useUploadStore.getState().uploadingFiles).toHaveLength(1)
    })

    it('leaves a failed row visible, so the error can be read', () => {
        const { add, clearDoneById } = useUploadStore.getState()
        add([entry({ id: 'a', status: 'error', errorMessage: 'too big' })])

        clearDoneById('a')

        expect(useUploadStore.getState().uploadingFiles).toHaveLength(1)
    })
})

describe('uploadProgress', () => {
    it('is the fraction of bytes sent', () => {
        expect(uploadProgress(entry({ size: 1000, loaded: 250 }))).toBe(0.25)
    })

    it('never exceeds 1, even if the server counts more bytes than the file', () => {
        expect(uploadProgress(entry({ size: 1000, loaded: 1200 }))).toBe(1)
    })

    it('reads as 0 for an unsized upload rather than dividing by zero', () => {
        expect(uploadProgress(entry({ size: 0, loaded: 10 }))).toBe(0)
    })
})
