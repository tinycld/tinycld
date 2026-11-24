import type { RecordLabelDecl } from '../record-labels/label'

/**
 * A record panel a package draws beside a report (manifest `reportPanels`, D5). A
 * report's detail row whose `open` is `<slug>/<path>/<segments…>` opens
 * `component` in the report's side panel on a window of 1360 px or more, else
 * pushes the path. `path` is one or more segments of `[A-Za-z0-9_-]` joined by
 * `/`. `component` is a module path under the package's `tinycld/<slug>/`,
 * resolved as `<package name>/<component>` through the package's exports map; its
 * default export is a React component that takes `ReportPanelProps`
 * (`@tinycld/core/reports/hooks/usePanelTitle`).
 */
export interface ReportPanelDecl {
    path: string
    component: string
}

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

    /**
     * Org-administration panels. Each package that declares any gets its own
     * group in the admin-only part of Settings, at
     * `/settings/<pkgSlug>/<slug>`. Only owners and admins can open them.
     */
    settings?: {
        slug: string
        component: string
        label: string
    }[]

    /**
     * Per-user panels: things a member does for their own account (import
     * their own data, connect their own integrations). Listed in the Account
     * group of Settings, at `/settings/account/<pkgSlug>/<slug>`, and open to
     * every role. Same shape and lazy-component resolution as `settings`.
     */
    accountSettings?: {
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
        /**
         * The system-settings key namespace this panel edits, dotted and
         * trailing-dot terminated (e.g. `'mail.'`).
         *
         * Declaring it lets the app hide this panel on a deployment whose
         * operator owns those values — the panel could not save there, because
         * the values never reach this deployment's database. Core matches on the
         * namespace rather than on the package, which is what lets it hide a
         * package's panel without knowing the package exists.
         *
         * Omit it and the panel is never hidden.
         */
        keyPrefix?: string
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
        label?: string
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

    server?: {
        package: string
        module: string
    }

    /**
     * Public TCP ports this package's server code listens on. The supervisor
     * that holds the public ports binds them once and passes them to each
     * server process.
     */
    ports?: PackagePort[]

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
     * Command-line commands this package contributes to the `tinycld` binary.
     * `package`/`module` mirror `server` and drive Go code generation
     * (scripts/gen-cli.ts): the dir must contain a Go module exposing
     * `Register(root *cobra.Command, c *client.Client)`.
     *
     * There is deliberately no command list here: Cobra owns the command tree
     * and `--help`, and a hand-maintained copy in the manifest only drifts.
     * OAuth scopes are likewise not declared here — the package's Go server
     * registers them (`oauth.RegisterPackage`), which is what the CLI asks
     * for at login and what the consent screen shows.
     */
    cli?: {
        package: string
        module: string
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

    /**
     * User-facing notification events this package emits: `events` is a
     * package-exports subpath to a pure-data module default-exporting
     * PackageNotifyEvents (event name → channels + variant). The same module
     * augments core's NotificationEvents interface with the events' payload
     * types, so `notify.emit` is typed in the package's own code. Core never
     * lists a package's events; the dispatcher composes them at boot.
     */
    notifications?: {
        events: string
    }

    /**
     * Form hosts: `hosts` is a package-exports subpath to a pure-data module
     * default-exporting FormHostDef[] (the generator imports it in Node);
     * `ui` is a subpath to the module default-exporting FormHostUi with the
     * components and collab hook. See
     * docs/superpowers/specs/2026-10-14-forms-engine-design.md.
     */
    forms?: {
        hosts: string
        ui: string
    }

    /**
     * How core's relation pickers (the rule builder's record picker, a form's
     * person field) name a record of this package's collections, keyed by
     * collection name. Without one, a picker shows the first of the fields
     * name, title, label, subject, display_name, email or username, else the
     * id. The picker's search matches the same text. See RecordLabelDecl.
     */
    recordLabels?: Record<string, RecordLabelDecl>

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

    /**
     * Steps this package adds to the first-run setup wizard. `module` is a
     * package-exports subpath whose module default-exports the step component
     * and may export `useIsStepDone` / `useIsStepVisible` (see
     * core/lib/setup/types.ts). `order` is a fractional-indexing key; core's
     * steps are a0 (workspace), a1 (apps), a2 (email), a3 (team).
     */
    setupSteps?: { id: string; label: string; module: string; order?: string }[]

    /**
     * Record panels this package draws beside a report (ReportPanelDecl). The
     * generator lists them in `lib/generated/report-panels.ts`.
     */
    reportPanels?: ReportPanelDecl[]

    /**
     * Terms and conditions the owner must accept before the package is used
     * in the app (PackageTermsDecl); the client gates a pending package. Omit it and the package needs no acceptance.
     */
    terms?: PackageTermsDecl
}

/**
 * A package's terms and conditions. `file` is a markdown file in the package,
 * relative to the package root (for example `'TERMS.md'`). `version` is a label
 * of 1 to 50 letters, digits, `.`, `_` or `-` that the author changes when the
 * text changes; core compares it for equality only. The owner accepts one
 * version: a new version blocks the package again until the owner accepts it.
 * The generator reads the file (scripts/gen-terms.ts), so the text is the text
 * of the installed package version.
 */
export interface PackageTermsDecl {
    file: string
    version: string
}

export interface PackagePort {
    /** Stable name the server code asks for, e.g. 'acme-sync'. [a-z0-9-]+ */
    name: string
    port: number
    /** Env var that overrides the listen address (host:port). */
    addrEnv?: string
    /**
     * Env var that turns the port on or off. default true: on unless the var
     * is 'false'; default false: on only if the var is 'true'.
     */
    enabled?: { env: string; default: boolean }
}
