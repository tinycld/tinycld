import { tinycldConfig } from '@tinycld/app-generated/tinycld-config'
import type { PackageManifest } from './types'

type EntryLike = { manifest: PackageManifest & { packageName?: string }; sidebar?: unknown }
type RegistryEntry = PackageManifest & { packageName: string }

/**
 * Map tinycld.config.ts entries to the registry shape usePackages() expects
 * (each manifest flattened with a guaranteed packageName). Replaces the old
 * generated package-registry.ts.
 *
 * A config entry's `sidebar` is a TOP-LEVEL field (sibling of `manifest`) — for
 * bundled packages it holds the file-derived component, while the manifest's own
 * `sidebar` is the authoring-time `{ component }` declaration bundled packages
 * never set. usePackage() consumers gate the package sidebar on
 * `activePkg?.sidebar != null`, so the registry must reflect the entry-level
 * presence; spreading only `e.manifest` dropped it and left every package
 * sidebar unrendered.
 */
export function toStaticRegistry(entries: readonly EntryLike[]): RegistryEntry[] {
    return entries.map(e => ({
        ...e.manifest,
        packageName: e.manifest.packageName ?? `@tinycld/${e.manifest.slug}`,
        sidebar:
            e.manifest.sidebar ??
            (e.sidebar != null ? { component: e.manifest.slug } : undefined),
    }))
}

/**
 * The statically-linked package set, derived once from tinycld.config.ts.
 * usePackages augments it with runtime-installed packages from the DB.
 */
export const packageRegistry = toStaticRegistry(tinycldConfig)
