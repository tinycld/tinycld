import type { PreviewRegistryEntry, PublicPreviewConfig, ShareEditorEntry } from './types'

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

// Public preview registry: maps a document mime type to the CSS surface
// and empty-state predicate needed to render that document's
// server-emitted HTML on the *anonymous* share page. Packages register
// only this thin config (not a whole component) so the share page's
// generic preview/comment rail can fetch via the share-session render
// endpoint without importing calc/text — same decoupling the action
// registry uses.
const publicPreviewRegistry = new Map<string, PublicPreviewConfig>()

export function registerPublicPreview(mimeType: string, config: PublicPreviewConfig) {
    publicPreviewRegistry.set(mimeType, config)
}

export function getPublicPreviewConfig(mimeType: string): PublicPreviewConfig | undefined {
    return publicPreviewRegistry.get(mimeType)
}

// Share-editor registry: maps a document mime to a full editor component
// the anonymous share route mounts from a prebuilt EditorMount. Packages
// register their real editor here so the drive share page can render it
// without a cross-module import (same decoupling as the preview registry).
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
    publicPreviewRegistry.clear()
    shareEditorRegistry.clear()
}
