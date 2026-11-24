// Vitest stub for lucide-react-native.
// Lucide's ESM bundle (v1.16+) loads individual icon .mjs files that contain
// Flow-style `typeof` syntax Vite/Rollup cannot parse. Returning a generic
// React component avoids the parse error while keeping the import shape valid
// for unit tests that don't assert icon rendering.
//
// The stub uses a Proxy so unknown icons resolve to the Icon function at runtime
// via get-trap. However, Vite's ESM interop for CJS modules uses property
// enumeration (not the get trap) to bind named imports — a Proxy with an empty
// target {} produces no enumerable properties, so named imports resolve to
// undefined in tests that check the value directly. We work around this by
// seeding the Proxy target with every icon name that any test file checks by
// value (not just renders as JSX). Add new names here when a test directly
// asserts the icon reference is non-null.
'use strict'

const React = require('react')

function Icon({ children, ...props }) {
    return null
}

// Check draws a marker so a test can tell a selected row from an unselected one.
function Check() {
    return React.createElement('span', { 'data-check': 'true' })
}

// Seed target with all icon names that unit tests reference by value (not just render).
// The Proxy's get-trap still handles any unknown name so JSX rendering always works.
const knownIcons = {
    // Forms list (core/components/forms-builder)
    Archive: Icon,
    ArchiveRestore: Icon,
    GripVertical: Icon,
    Star: Icon,
    // Table row reorder buttons (core/ui/forms/fields/TableField.tsx)
    ArrowDown: Icon,
    ArrowUp: Icon,
    // Toolbar icons (core/tests/unit/responsive-toolbar.test.tsx)
    Bold: Icon,
    Ellipsis: Icon,
    SquareKanban: Icon,
    Shield: Icon,
    Check: Icon,
    Calendar: Icon,
    User: Icon,
    Server: Icon,
    LogOut: Icon,
    Bell: Icon,
    Settings: Icon,
    PanelLeftOpen: Icon,
    PanelLeftClose: Icon,
    Building2: Icon,
    EllipsisVertical: Icon,
    Italic: Icon,
    Underline: Icon,
    // Toast variant icons (core/components/Toast.tsx)
    CheckCircle: Icon,
    Info: Icon,
    XCircle: Icon,
    // lucide-react-native named exports used by slash-menu-icon-lookup.ts and similar
    Code2: Icon,
    Heading1: Icon,
    Heading2: Icon,
    Heading3: Icon,
    Image: Icon,
    List: Icon,
    ListOrdered: Icon,
    Minus: Icon,
    Plus: Icon,
    Quote: Icon,
    Table: Icon,
    Type: Icon,
    // Settings screens mounted whole (tests/audit-log-query.mount.test.tsx)
    ArrowLeft: Icon,
    // Additional icons imported by text components
    AlertCircle: Icon,
    ChevronRight: Icon,
    AlertTriangle: Icon,
    AlignJustify: Icon,
    Ban: Icon,
    Check,
    Square: Icon,
    CheckSquare: Icon,
    CalendarDays: Icon,
    ChevronDown: Icon,
    ChevronUp: Icon,
    HelpCircle: Icon,
    CloudOff: Icon,
    ExternalLink: Icon,
    FilePlus2: Icon,
    FileSpreadsheet: Icon,
    FileText: Icon,
    LayoutTemplate: Icon,
    Mail: Icon,
    MessageSquare: Icon,
    MessageSquarePlus: Icon,
    Pencil: Icon,
    Replace: Icon,
    ReplaceAll: Icon,
    RotateCcw: Icon,
    Rows3: Icon,
    ScrollText: Icon,
    Upload: Icon,
    WrapText: Icon,
    X: Icon,
    // calc and other package icons
    FilePlus: Icon,
    Plus: Icon,
    // boards: CardHeaderToolbar's overflow table holds the icon by value
    Maximize2: Icon,
    // boards: ColumnMenu, mounted whole (tests/column-menu.mount.test.tsx);
    // ArrowLeft, FilePlus, Pencil, Plus and Trash2 are seeded elsewhere
    ArrowRight: Icon,
    FoldHorizontal: Icon,
    Gauge: Icon,
    MoreHorizontal: Icon,
    // boards: CardHeaderToolbar, CardActionsMenu and ColumnMenu's icon tables
    ArrowRightLeft: Icon,
    Expand: Icon,
    Link: Icon,
    Link2: Icon,
    PanelRight: Icon,
    Shapes: Icon,
    // drive icons (if any are value-checked)
    FolderOpen: Icon,
    Folder: Icon,
    // boards: pr-link-status.ts's state → icon table (tests/pr-link-chip.test.ts)
    GitMerge: Icon,
    GitPullRequest: Icon,
    GitPullRequestClosed: Icon,
    // boards: PriorityGlyph's priority → icon table (tests/form-fields.mount.test.tsx)
    OctagonAlert: Icon,
    SignalHigh: Icon,
    SignalLow: Icon,
    SignalMedium: Icon,
    // boards: the card detail body, mounted whole (tests/card-detail.mount.test.tsx)
    SmilePlus: Icon,
    // boards: the sub-task row's state glyphs, mounted whole
    CircleCheck: Icon,
    CircleDashed: Icon,
    // oauth: ConnectedAppsSection's revoke button
    Trash2: Icon,
    // Package settings screens mounted by member tests.
    Copy: Icon,
    Globe: Icon,
    Loader2: Icon,
    RefreshCw: Icon,
    File: Icon,
    AtSign: Icon,
    // Group admin drawer (core/tests/unit/group-drawer-removal.test.tsx)
    Search: Icon,
    UserPlus: Icon,
    // A package's list-icon fallback, compared by value in its unit test
    BookOpen: Icon,
    // A package's shortcut rows: report and list icons, compared by value
    BarChart3: Icon,
    Package: Icon,
    Truck: Icon,
    Users: Icon,
    // Group share section (core/components/groups/GroupShareSection.tsx)
    UsersRound: Icon,
    // core Toast's VARIANT_ICONS table (AlertTriangle and X are seeded above)
    CheckCircle: Icon,
    Info: Icon,
    XCircle: Icon,
}

const handler = {
    get(target, prop) {
        if (prop === '__esModule') return true
        if (prop === 'default') return Icon
        if (prop === 'LucideProvider') {
            return function LucideProvider({ children }) {
                return children
            }
        }
        if (prop === 'useLucideContext') {
            return function useLucideContext() {
                return { size: 24, color: 'currentColor', strokeWidth: 2, nonScalingStroke: false }
            }
        }
        if (prop === 'createLucideIcon') {
            return function createLucideIcon(_name, _iconNode) {
                return Icon
            }
        }
        // The trap ignores the seeded target, so Check needs its own branch.
        if (prop === 'Check') return Check
        // Any named icon export (ChevronDown, Plus, etc.)
        return Icon
    },
}

module.exports = new Proxy(knownIcons, handler)
