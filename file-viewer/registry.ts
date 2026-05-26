import type { PreviewRegistryEntry, ShareEditorEntry } from './types'

const registry = new Map<string, PreviewRegistryEntry>()

export function registerPreview(pattern: string, entry: PreviewRegistryEntry) {
    registry.set(pattern, entry)
}

export function getPreviewEntry(mimeType: string): PreviewRegistryEntry | undefined {
    const exact = registry.get(mimeType)
    if (exact) return exact

    const wildcard = `${mimeType.split('/')[0]}/*`
    const wildcardEntry = registry.get(wildcard)
    if (wildcardEntry) return wildcardEntry

    return registry.get('*')
}

// Share-editor registry: maps a document mime to a full editor component
// the anonymous share route mounts from a prebuilt EditorMount. Packages
// register their real editor here so the drive share page can render it
// without a cross-module import.
const shareEditorRegistry = new Map<string, ShareEditorEntry>()

export function registerShareEditor(mimeType: string, entry: ShareEditorEntry) {
    shareEditorRegistry.set(mimeType, entry)
}

export function getShareEditor(mimeType: string): ShareEditorEntry | undefined {
    return shareEditorRegistry.get(mimeType)
}

/** Test-only: drop all registrations. */
export function __resetRegistryForTests() {
    registry.clear()
    shareEditorRegistry.clear()
}
