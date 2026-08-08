/** One rendered row in the palette. Every adapter maps its hits to this. */
export interface SearchRow {
    /** The package slug this row came from. Set by the palette, not the adapter. */
    slug: string
    /** Record id, unique within the package. */
    id: string
    /** The name a user would recognize — file name, subject, card title. */
    title: string
    /** Identifying detail, e.g. 'Grace Hopper · Inbox · 1d'. */
    subtitle?: string
    /** Right-aligned trailing detail, e.g. a board name. */
    meta?: string
}

/**
 * What an adapter module exports. Two halves because rendering is pure but
 * selection needs router and store handles.
 */
export interface SearchAdapterModule {
    /**
     * Returns this package's selection handler.
     *
     * MUST be side-effect free: the palette calls every in-scope package's
     * hook at the top level (hooks cannot be called conditionally at selection
     * time), so this runs even for packages with no visible results. Wire up
     * router/store handles here — never fetch, subscribe or mutate.
     */
    useSearchActions: () => { onSelect: (row: SearchRow) => void }
}

/** The result of parsing the palette input. */
export interface ParsedQuery {
    /** Package slugs to search. Empty = every package declaring `search`. */
    chips: string[]
    /** Terms that must match. */
    include: string[]
    /** Terms that must NOT match. */
    exclude: string[]
    /**
     * The exact free-text remainder to display in the input box: a substring
     * of the raw input starting right after the last recognized chip token.
     * The renderer must use this rather than re-deriving it (e.g. by slicing
     * on `chipsToText(chips).length`) — that computed-length approach assumes
     * chips are a leading prefix, which breaks the moment a chip is created
     * after free text already typed.
     */
    remainder: string
}

/** A package the palette can search, derived from the manifest registry. */
export interface SearchPackage {
    slug: string
    label: string
    icon: string
    order: number
}
