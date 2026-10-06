import { withNativeFileParts } from '@tinycld/core/lib/multipart-body'
import type { Fetch } from '@tinycld/core/lib/read-only-retry'

// expo-file-system is imported on first use, the way core's file-url.ts does,
// so a module-init failure there cannot take down every screen that imports
// the PocketBase client.
async function readFile(uri: string): Promise<Uint8Array> {
    const { File } = await import('expo-file-system')
    return new Uint8Array(await new File(uri).arrayBuffer())
}

// React Native's Blob has no arrayBuffer(); its FileReader reads one from the
// native blob store instead.
function readBlob(blob: Blob): Promise<Uint8Array> {
    if (typeof blob.arrayBuffer === 'function') {
        return blob.arrayBuffer().then(buffer => new Uint8Array(buffer))
    }
    return new Promise((resolve, reject) => {
        const reader = new FileReader()
        reader.onload = () => {
            if (reader.result instanceof ArrayBuffer) resolve(new Uint8Array(reader.result))
            else reject(new Error('FileReader did not return an ArrayBuffer'))
        }
        reader.onerror = () => reject(reader.error ?? new Error('Could not read the Blob'))
        reader.readAsArrayBuffer(blob)
    })
}

export function withNativeUploadBodies(fetchImpl: Fetch): Fetch {
    return withNativeFileParts(fetchImpl, { readFile, readBlob })
}
