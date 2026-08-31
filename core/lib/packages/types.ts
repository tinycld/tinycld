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
     * System-wide settings panels this package contributes to the /admin
     * Settings section (deployment/system scope — NOT org-scoped like
     * `settings`). Same shape and lazy-component resolution as `settings`:
     * `component` is a package-exports subpath (e.g.
     * `'system-settings/provider'`). Used for "use-your-own-service" config a
     * host owns (mail provider creds, etc.). Rendered only in the super-admin
     * console; values are stored system-wide, not per org.
     */
    systemSettings?: {
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

    /**
     * Read-only event feeds this package contributes to another package's
     * event grid (today: the calendar). `target` is the host package slug —
     * the host must declare `eventSourceHost: true` or generation fails;
     * an absent host leaves the source silently inactive (lean shell).
     * `id` is unique per target and restricted to `[a-z0-9-]` (the host embeds
     * it in `:`-delimited synthetic event ids). `module` is a package-exports
     * subpath to a module exporting `useEventSource` — see
     * `core/lib/event-sources/types.ts` for the contract. Items are read-only
     * on the host grid: no drag, no edit; a press navigates to the item's
     * `href`. `color` names a calendar color key the host resolves.
     */
    eventSources?: {
        target: string
        id: string
        label: string
        module: string
        color?: string
        order?: number
    }[]

    /**
     * Declares this package hosts event-source contributions: it mounts the
     * collectors, merges contributed items into its grid, and renders the
     * per-source visibility toggles. Contributions targeting a package
     * without this flag fail generation.
     */
    eventSourceHost?: boolean

    seed?: {
        script: string
    }

    tests?: {
        directory: string
    }

    /**
     * `mailListeners`: this package serves mail protocols, so the hosting
     * ROUTER creates per-org mail sockets for orgs whose set includes it; the
     * package's single Register discovers them via coreserver's TenantContext
     * (host mode binds its own ports instead).
     */
    server?: {
        package: string
        module: string
        mailListeners?: boolean
    }

    /**
     * Go payload package (dir relative to the member root, e.g. 'server/api')
     * holding the exported HTTP request/response structs for this package's
     * API. The generator emits lib/generated/<slug>-api.ts from it, so the
     * web client (and the CLI) consume the Go structs as the single source
     * of truth for payload shapes.
     */
    payloads?: {
        package: string
    }

    /**
     * Protocol capabilities. Core serves these; a package contributes only the
     * config, so a hosting tenant (which links no feature Go) still gets the
     * protocol. The host materializes these blocks into the tenant's runtime
     * config — see hosting's controlplane/capabilities.go, which mirrors
     * every shape here.
     */
    carddav?: {
        collection: string
        listFilter: string
        sort?: string
        ownerField: string
        uidField: string
        softDeleteField?: string
        vcard: {
            version: string
            name: { given: string; family: string }
            simple: Record<string, string>
            revField?: string
        }
    }

    caldav?: {
        prefix?: string
        calendarCollection: string
        eventCollection: string
        calendar: {
            name: string
            description?: string
        }
        event: {
            calendar: string
            uid: string
            owner: string
            title: string
            description?: string
            location?: string
            start: string
            end: string
            allDay?: string
            recurrence?: string
            guests?: string
            reminder?: string
            busyStatus?: string
            visibility?: string
            updated?: string
            created?: string
            /**
             * Values for required select fields a minimal client payload
             * omits (e.g. a bare VEVENT carries neither TRANSP nor CLASS).
             */
            defaults?: Record<string, string>
        }
    }

    webdav?: {
        prefix: string
        collection: string
        fields: {
            name: string
            parent: string
            isFolder: string
            size: string
            file: string
            owner: string
            mimeType?: string
            updated?: string
        }
        /**
         * Binds the feature's per-user soft-delete state; when set, a DAV
         * DELETE stamps it instead of destroying the record.
         */
        trash?: {
            collection: string
            itemField: string
            userField: string
            trashedAtField: string
        }
    }

    /**
     * Storage-bearing collections. core/quota enforces the ceilings from this
     * as record hooks, so no write path can skip them. A source with no
     * ownerField counts toward the org ceiling only.
     */
    quota?: {
        collection: string
        sizeField: string
        ownerField?: string
    }[]

    /**
     * Command-line commands this package contributes to the `tinycld` binary.
     * `package`/`module` mirror `server` and drive Go code generation
     * (scripts/gen-cli.ts): the dir must contain a Go module exposing
     * `Register(root *cobra.Command, c *client.Client)`.
     *
     * `scopes` declares the OAuth scopes this package defines (e.g.
     * `'mail:read'`) for the scope registry and consent screen; it is never
     * interpolated into generated Go. There is deliberately no command list
     * here: Cobra owns the command tree and `--help`, and a hand-maintained
     * copy in the manifest only drifts.
     */
    cli?: {
        package: string
        module: string
        scopes?: string[]
    }

    help?: {
        directory: string
    }

    /**
     * How this package participates in the global `/` search palette.
     * `adapter` is a package-exports subpath to a module exporting
     * `useSearchActions`, the client-side selection handler. Omit to stay out
     * of the palette.
     *
     * There is deliberately no endpoint: rows come from core's federated
     * /api/search, which reads the package's Go-registered search source. A
     * per-package route would have no reader.
     */
    search?: {
        adapter: string
        /** Chip and group label. Defaults to `nav.label`. */
        label?: string
    }

    /**
     * Workflow-rules catalog. `definitions` is a package-exports subpath (like
     * `seed.script`) resolving to a TS module default-exporting an
     * AutomationDefinitions object — pure data, typed against the package's
     * schema. See docs/superpowers/specs/2026-08-11-workflow-rules-design.md.
     */
    automation?: {
        definitions: string
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
