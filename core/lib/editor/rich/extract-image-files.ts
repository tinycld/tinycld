// Pure helpers extracting image File objects from drop / paste events, kept
// outside the editor hook so they unit-test under node without the
// Tiptap/ProseMirror DOM stack. Duck-typed event shapes for the same reason;
// real DOM events satisfy them implicitly. (Mirrors text's
// lib/extract-image-files.ts — the rich editor cannot import from a sibling.)

export interface DragEventLike {
    dataTransfer: {
        files?: ArrayLike<File> | null
    } | null
}

export interface ClipboardEventLike {
    clipboardData: {
        items?: ArrayLike<DataTransferItemLike> | null
    } | null
}

export interface DataTransferItemLike {
    kind: string
    type: string
    getAsFile: () => File | null
}

export function extractImageFilesFromDrop(event: DragEventLike): File[] {
    const files = event.dataTransfer?.files
    if (files == null) return []
    const out: File[] = []
    for (let i = 0; i < files.length; i++) {
        const file = files[i]
        if (file == null) continue
        if (!file.type.startsWith('image/')) continue
        out.push(file)
    }
    return out
}

export function extractImageFilesFromPaste(event: ClipboardEventLike): File[] {
    const items = event.clipboardData?.items
    if (items == null) return []
    const files: File[] = []
    for (let i = 0; i < items.length; i++) {
        const item = items[i]
        if (item == null) continue
        if (item.kind !== 'file') continue
        if (!item.type.startsWith('image/')) continue
        const file = item.getAsFile()
        if (file != null) files.push(file)
    }
    return files
}
