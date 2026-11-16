import { errorToString } from '@tinycld/core/lib/errors'
import { log } from '@tinycld/core/lib/logger'
import type { Directory as CacheDirectory } from 'expo-file-system'
import { Platform } from 'react-native'

export interface SaveFileInput {
    bytes: Uint8Array<ArrayBuffer>
    /** The saved file's name, extension included. */
    fileName: string
    mimeType: string
    /** The iOS Uniform Type Identifier the share sheet offers the file as (`com.adobe.pdf`). */
    uti?: string
}

/**
 * Save bytes the app fetched as a file: a Blob URL and an anchor click on web,
 * a cache file and the share sheet on native. The caller fetches the bytes
 * itself (with the bearer token a route needs), because a browser anchor or the
 * native downloader would make its own unauthenticated request.
 */
export async function saveFile({ bytes, fileName, mimeType, uti }: SaveFileInput): Promise<void> {
    // Checked here, not only in fileNameFromDisposition: a caller can pass any
    // name, and on native the name becomes a path inside the download directory.
    if (!isSafeFileName(fileName)) throw new Error('Unsafe file name')
    if (Platform.OS === 'web') {
        const href = URL.createObjectURL(new Blob([bytes], { type: mimeType }))
        const anchor = document.createElement('a')
        anchor.href = href
        anchor.download = fileName
        anchor.click()
        // Revoked on the next tick: Safari has not read the blob when click() returns.
        setTimeout(() => URL.revokeObjectURL(href), 0)
        return
    }
    // Lazily imported, as core file-url.ts does: a module-init failure must not
    // break every screen that merely shows a download button.
    const [{ Directory, File, Paths }, Sharing] = await Promise.all([
        import('expo-file-system'),
        import('expo-sharing'),
    ])
    if (!(await Sharing.isAvailableAsync())) {
        throw new Error('Sharing is not available on this device')
    }
    removeEarlierDownloads(Directory, Paths.cache)
    // A unique directory so repeated downloads do not collide while the shared name stays readable.
    const dir = new Directory(Paths.cache, `${DOWNLOAD_DIR_PREFIX}${Date.now()}`)
    dir.create({ intermediates: true, idempotent: true })
    const target = new File(dir, fileName)
    target.create()
    await target.write(bytes)
    await Sharing.shareAsync(target.uri, { mimeType, UTI: uti, dialogTitle: fileName })
}

const DOWNLOAD_DIR_PREFIX = 'tinycld-download-'

// The share sheet can still be reading a file when it resolves, so a download is
// deleted by the next one, not by itself: at most one directory remains.
function removeEarlierDownloads(DirectoryClass: typeof CacheDirectory, cache: CacheDirectory) {
    for (const entry of cache.list()) {
        if (!(entry instanceof DirectoryClass) || !entry.name.startsWith(DOWNLOAD_DIR_PREFIX))
            continue
        try {
            entry.delete()
        } catch (error) {
            log.warn('core.save-file', 'could not delete an earlier download', {
                error: errorToString(error),
            })
        }
    }
}

// The file name the server gave in its Content-Disposition header (RFC 6266):
// the UTF-8 filename* when present and decodable, else filename. null when the
// header has no usable name, or the name could leave its directory.
export function fileNameFromDisposition(header: string | null): string | null {
    if (!header) return null
    const extended = /(?:^|;)\s*filename\*\s*=\s*([^;]+)/i.exec(header)?.[1]?.trim()
    const encoded = extended ? /^UTF-8'[^']*'(.+)$/i.exec(extended)?.[1] : undefined
    if (encoded) {
        const decoded = decodePercent(encoded)
        if (decoded !== null && isSafeFileName(decoded)) return decoded
    }
    const plain = /(?:^|;)\s*filename\s*=\s*(?:"((?:\\.|[^"\\])*)"|([^;\s]+))/i.exec(header)
    const name = plain?.[1]?.replace(/\\(.)/g, '$1') ?? plain?.[2]
    return name !== undefined && isSafeFileName(name) ? name : null
}

function decodePercent(text: string): string | null {
    try {
        return decodeURIComponent(text)
    } catch {
        return null
    }
}

// The server already cleans the name; this guards the native file write
// against a header from anything else and a caller that passes its own name.
function isSafeFileName(name: string): boolean {
    const trimmed = name.trim()
    // Compared by code point: biome refuses control characters in a regex.
    const hasControl = [...trimmed].some(ch => {
        const code = ch.codePointAt(0) ?? 0
        return code < 0x20 || code === 0x7f
    })
    return (
        trimmed !== '' &&
        trimmed !== '.' &&
        trimmed !== '..' &&
        !/[/\\]/.test(trimmed) &&
        !hasControl
    )
}
