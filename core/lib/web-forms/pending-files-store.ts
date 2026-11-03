import type { PickedFile } from '@tinycld/core/file-viewer/picked-file'
import { create } from '@tinycld/core/lib/store'
import { newRecordId } from 'pbtsdb/core'

interface PendingFilesState {
    files: Record<string, PickedFile>
    add: (picked: PickedFile[]) => string[]
    remove: (id: string) => void
    clear: () => void
}

/**
 * Files a visitor picked on a public form, held until submit. The form's
 * value for the field is the local ids, so required checks work as for any
 * file field; the submit hook reads the files back by id.
 */
export const usePendingFiles = create<PendingFilesState>()(set => ({
    files: {},
    add: picked => {
        const entries = picked.map(file => [newRecordId(), file] as const)
        set(s => ({ files: { ...s.files, ...Object.fromEntries(entries) } }))
        return entries.map(([id]) => id)
    },
    remove: id =>
        set(s => {
            const { [id]: _gone, ...rest } = s.files
            return { files: rest }
        }),
    clear: () => set({ files: {} }),
}))
