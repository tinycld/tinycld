// Self-contained RFC 4122 v4 UUID built straight from crypto.getRandomValues.
// Unlike expo-crypto's web randomUUID — which delegates back to
// globalThis.crypto.randomUUID and so infinitely recurses if it's installed as
// that very property — this depends only on getRandomValues (always present
// once react-native-get-random-values has run, and on every browser), so it is
// safe to assign as crypto.randomUUID on any platform, including web served
// over a non-secure (plain http, non-localhost) origin where the browser does
// not expose a native crypto.randomUUID.
export function uuidFromRandomValues(): `${string}-${string}-${string}-${string}-${string}` {
    const bytes = new Uint8Array(16)
    crypto.getRandomValues(bytes)
    bytes[6] = (bytes[6] & 0x0f) | 0x40
    bytes[8] = (bytes[8] & 0x3f) | 0x80
    const hex = Array.from(bytes).map(b => b.toString(16).padStart(2, '0'))
    return `${hex.slice(0, 4).join('')}-${hex.slice(4, 6).join('')}-${hex.slice(6, 8).join('')}-${hex.slice(8, 10).join('')}-${hex.slice(10).join('')}`
}
