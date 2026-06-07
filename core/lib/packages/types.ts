export interface PackageManifest {
    name: string
    slug: string
    version: string
    description: string

    routes?: {
        directory: string
    }

    publicRoutes?: {
        directory: string
    }

    nav?: {
        label: string
        icon: string
        order?: number
        /**
         * Single character that registers a `t <letter>` jump to this
         * package's root screen. Must be unique across installed packages
         * (validated at generation time).
         */
        shortcut?: string
    }

    migrations?: {
        directory: string
    }

    hooks?: {
        directory: string
    }

    collections?: {
        register: string
        types: string
    }

    sidebar?: {
        component: string
    }

    settings?: {
        slug: string
        component: string
        label: string
    }[]

    /**
     * Names of sidebar slots this package exposes. Other packages target these
     * names from their `sidebarContributions`. Each name must be unique within
     * the manifest; the generator errors on duplicates and on contributions
     * targeting an unknown slot.
     */
    slots?: string[]

    /**
     * UI contributions this package injects into another package's sidebar
     * slot. `target` is the host package slug; `slot` is one of the names the
     * host declares in its own `slots` array; `component` is a package-exports
     * subpath (e.g. `'sidebar-contributions/booking-pages'`) resolved through
     * the same lazy-import path used for `sidebar` and `settings`.
     */
    sidebarContributions?: {
        target: string
        slot: string
        component: string
        order?: number
    }[]

    seed?: {
        script: string
    }

    tests?: {
        directory: string
    }

    server?: {
        package: string
        module: string
    }

    help?: {
        directory: string
    }

    repository?: {
        url: string
        issueTemplate?: string
    }

    /**
     * Subpath (relative to the package root, resolved through the package's
     * exports map) to a TS module exporting a default async function. The
     * generator runs every declared build script before emitting generated
     * files; `dev` keeps the process alive in --watch mode for incremental
     * rebuilds. Use for build artifacts the package's runtime code needs
     * but that the bundler can't produce on its own (e.g. an embedded
     * webview bundle).
     */
    build?: {
        script: string
    }

    /**
     * Optional component (rendered from `provider`) that wraps app children
     * with package-supplied context. Resolved through the exports map like
     * other component fields.
     */
    provider?: {
        component: string
    }

    dependencies?: string[]

    /**
     * Semver ranges this version of the package requires of OTHER packages
     * (and of `@tinycld/core`) to be compatible. Keys are package slugs or
     * `@tinycld/core`; values are semver ranges (e.g. `'>=2.1 <3'`, `'^1.0'`).
     *
     * The version-management compatibility solver checks a proposed set of
     * version changes against every selected package's `peerVersions`: a change
     * is blocked if it would leave any declared range unsatisfied by the
     * resolved versions. Unlike `dependencies` (which is advisory + seed
     * ordering and only names slugs), `peerVersions` is enforced and version-aware.
     * Omit it when a package has no cross-package version constraints.
     */
    peerVersions?: Record<string, string>
}
