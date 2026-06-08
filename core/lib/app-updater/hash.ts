import * as Crypto from 'expo-crypto'
// SDK 55 moved the URI-string file API (readAsStringAsync, EncodingType) to the
// `/legacy` entry; the new File/Directory API is a different shape. The legacy
// surface is exactly what the byte-level hashing here needs.
import * as FileSystem from 'expo-file-system/legacy'

const HEX = '0123456789abcdef'

// Decode base64 text to its raw bytes. expo-file-system has no raw-bytes read
// API, so we read base64 and decode here. Hermes provides atob, but we decode
// manually to avoid depending on it being present on every RN runtime.
function base64ToBytes(base64: string): Uint8Array<ArrayBuffer> {
    const lookup = new Int8Array(256).fill(-1)
    const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'
    for (let i = 0; i < alphabet.length; i++) {
        lookup[alphabet.charCodeAt(i)] = i
    }

    let clean = base64
    const padIndex = clean.indexOf('=')
    if (padIndex !== -1) clean = clean.slice(0, padIndex)

    const byteLength = Math.floor((clean.length * 3) / 4)
    const bytes = new Uint8Array(new ArrayBuffer(byteLength))

    let bitBuffer = 0
    let bitCount = 0
    let outIndex = 0
    for (let i = 0; i < clean.length; i++) {
        const value = lookup[clean.charCodeAt(i)]
        if (value === -1) continue
        bitBuffer = (bitBuffer << 6) | value
        bitCount += 6
        if (bitCount >= 8) {
            bitCount -= 8
            bytes[outIndex++] = (bitBuffer >> bitCount) & 0xff
        }
    }
    return bytes
}

function bytesToHex(bytes: Uint8Array): string {
    let hex = ''
    for (let i = 0; i < bytes.length; i++) {
        const b = bytes[i]
        hex += HEX[(b >> 4) & 0x0f] + HEX[b & 0x0f]
    }
    return hex
}

// Lowercase hex SHA-256 of a file's raw bytes — matches the server's
// hex.EncodeToString(sha256(file)). Reads base64 (expo-file-system has no raw
// bytes API), decodes to bytes, hashes via expo-crypto's digest() which hashes
// the supplied BufferSource directly (NOT the string, so the base64 text is
// never what gets hashed).
export async function sha256HexOfFile(uri: string): Promise<string> {
    const base64 = await FileSystem.readAsStringAsync(uri, {
        encoding: FileSystem.EncodingType.Base64,
    })
    const bytes = base64ToBytes(base64)
    // bytes is backed by a definite ArrayBuffer (see base64ToBytes), so the view
    // is a valid BufferSource under TS's stricter lib typings.
    const digest = await Crypto.digest(Crypto.CryptoDigestAlgorithm.SHA256, bytes)
    return bytesToHex(new Uint8Array(digest))
}
