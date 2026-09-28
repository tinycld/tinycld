import { eq, like, or } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import {
    AUDIT_PAGE_SIZE,
    auditWindowSize,
    hasMoreAuditPages,
} from '@tinycld/core/lib/audit-log-page'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useDebouncedValue } from '@tinycld/core/lib/use-debounced-value'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { ArrowLeft, ChevronDown, ChevronUp } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'

const SEARCH_DEBOUNCE_MS = 250

const ACTION_OPTIONS = [
    { label: 'All', value: '' },
    { label: 'Created', value: 'created' },
    { label: 'Updated', value: 'updated' },
    { label: 'Deleted', value: 'deleted' },
    { label: 'Backup created', value: 'backup.created' },
    { label: 'Backup failed', value: 'backup.failed' },
    { label: 'Restore started', value: 'restore.started' },
    { label: 'Restore succeeded', value: 'restore.succeeded' },
    { label: 'Restore failed', value: 'restore.failed' },
] as const

const RESOURCE_TYPE_OPTIONS = [
    { label: 'All', value: '' },
    { label: 'Contacts', value: 'contacts' },
    { label: 'Calendar', value: 'calendar_events' },
    { label: 'Calendars', value: 'calendar_calendars' },
    { label: 'Drive', value: 'drive_items' },
    { label: 'Mail', value: 'mail_messages' },
    { label: 'Mailboxes', value: 'mail_mailboxes' },
    { label: 'Domains', value: 'mail_domains' },
    // Single-org: membership lives on the users record, and every writer
    // (leave_org.go, users_guard.go) stamps resource_type='users'. Filtering
    // on the deleted junction's name matched nothing, so this option silently
    // returned an empty list.
    { label: 'Members', value: 'users' },
    { label: 'Labels', value: 'labels' },
    { label: 'Settings', value: 'settings' },
    { label: 'Packages', value: 'pkg_registry' },
] as const

// Each audit action maps to a semantic soft-status class pair so the badge
// tracks the theme (created=success, updated=accent, deleted=danger) instead of
// a fixed hex that would read the same — and clash — in dark mode.
const ACTION_BADGE_CLASS = {
    created: 'bg-success-soft',
    updated: 'bg-accent',
    deleted: 'bg-danger-soft',
    'backup.created': 'bg-success-soft',
    'backup.failed': 'bg-danger-soft',
    'restore.started': 'bg-accent',
    'restore.succeeded': 'bg-success-soft',
    'restore.failed': 'bg-danger-soft',
} as const
const ACTION_TEXT_CLASS = {
    created: 'text-success-soft-foreground',
    updated: 'text-accent-foreground',
    deleted: 'text-danger-soft-foreground',
    'backup.created': 'text-success-soft-foreground',
    'backup.failed': 'text-danger-soft-foreground',
    'restore.started': 'text-accent-foreground',
    'restore.succeeded': 'text-success-soft-foreground',
    'restore.failed': 'text-danger-soft-foreground',
} as const

type AuditAction = keyof typeof ACTION_BADGE_CLASS

export default function AuditLogSettings() {
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const { isAdmin } = useCurrentRole()
    const [actionFilter, setActionFilter] = useState('')
    const [resourceFilter, setResourceFilter] = useState('')
    const [search, setSearch] = useState('')
    const debouncedSearch = useDebouncedValue(search.trim(), SEARCH_DEBOUNCE_MS)

    const fgColor = useThemeColor('foreground')

    if (!isAdmin) {
        return (
            <View className="flex-1 p-5 items-center justify-center bg-background">
                <DocumentTitle pkg="Settings" title="Audit log" />
                <Text className="text-muted-foreground text-base">
                    Only admins can view audit logs.
                </Text>
            </View>
        )
    }

    return (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} className="bg-background">
            <DocumentTitle pkg="Settings" title="Audit log" />
            <View className="flex-1 p-5 max-w-[700px]">
                <View className="flex-row gap-3 items-center mb-5">
                    <Pressable onPress={navigateBack}>
                        <ArrowLeft size={24} color={fgColor} />
                    </Pressable>
                    <Text className="text-foreground text-[22px] font-bold">Audit Log</Text>
                </View>

                <FilterBar
                    actionFilter={actionFilter}
                    onActionChange={setActionFilter}
                    resourceFilter={resourceFilter}
                    onResourceChange={setResourceFilter}
                    search={search}
                    onSearchChange={setSearch}
                />

                {/* Keyed on the query terms so a filter or search change remounts
                    the list and drops back to page 1 — otherwise a narrowed
                    search would keep asking for the window the old, wider query
                    had grown to. */}
                <AuditLogList
                    key={`${actionFilter}|${resourceFilter}|${debouncedSearch}`}
                    actionFilter={actionFilter}
                    resourceFilter={resourceFilter}
                    search={debouncedSearch}
                />
            </View>
        </ScrollView>
    )
}

function FilterBar({
    actionFilter,
    onActionChange,
    resourceFilter,
    onResourceChange,
    search,
    onSearchChange,
}: {
    actionFilter: string
    onActionChange: (v: string) => void
    resourceFilter: string
    onResourceChange: (v: string) => void
    search: string
    onSearchChange: (v: string) => void
}) {
    const placeholderColor = useThemeColor('field-placeholder')
    return (
        <View className="mb-4 gap-3">
            <View className="gap-1.5">
                <FilterLabel text="Search" />
                <PlainInput
                    value={search}
                    onChangeText={onSearchChange}
                    placeholder="Resource, action or type…"
                    placeholderTextColor={placeholderColor}
                    testID="audit-search"
                    className="border rounded-lg px-2.5 py-1.5 text-sm text-foreground bg-background border-border"
                />
            </View>
            <View className="gap-1.5">
                <FilterLabel text="Action" />
                <View className="flex-row gap-1.5 flex-wrap">
                    {ACTION_OPTIONS.map(opt => (
                        <FilterChip
                            key={opt.value}
                            label={opt.label}
                            isActive={actionFilter === opt.value}
                            onPress={() => onActionChange(opt.value)}
                        />
                    ))}
                </View>
            </View>
            <View className="gap-1.5">
                <FilterLabel text="Resource" />
                <View className="flex-row gap-1.5 flex-wrap">
                    {RESOURCE_TYPE_OPTIONS.map(opt => (
                        <FilterChip
                            key={opt.value}
                            label={opt.label}
                            isActive={resourceFilter === opt.value}
                            onPress={() => onResourceChange(opt.value)}
                        />
                    ))}
                </View>
            </View>
        </View>
    )
}

function FilterLabel({ text }: { text: string }) {
    return <Text className="text-muted-foreground text-xs font-semibold">{text}</Text>
}

function FilterChip({
    label,
    isActive,
    onPress,
}: {
    label: string
    isActive: boolean
    onPress: () => void
}) {
    return (
        <Pressable
            onPress={onPress}
            className={`px-2.5 py-1 rounded-md ${isActive ? 'bg-primary' : 'border border-border'}`}
        >
            <Text className={`text-xs ${isActive ? 'text-primary-foreground' : 'text-primary'}`}>
                {label}
            </Text>
        </Pressable>
    )
}

function AuditLogList({
    actionFilter,
    resourceFilter,
    search,
}: {
    actionFilter: string
    resourceFilter: string
    search: string
}) {
    const [auditLogsCollection, usersCollection] = useStore('audit_logs', 'users')
    // Paging position is synchronous, screen-local UI state — nothing else reads
    // it. The parent keys this component on the query terms, so a filter or
    // search change remounts it and the window returns to one page.
    const [page, setPage] = useState(1)

    const { data: logs } = useLiveQuery({
        query: query => {
            let q = query.from({ audit_logs: auditLogsCollection })
            // Every predicate goes into the request. `audit_logs` is unbounded
            // history, so a client-side filter over an unfiltered fetch would
            // download a deployment's entire history to render fifty rows.
            if (actionFilter) {
                q = q.where(({ audit_logs }) => eq(audit_logs.action, actionFilter))
            }
            if (resourceFilter) {
                q = q.where(({ audit_logs }) => eq(audit_logs.resource_type, resourceFilter))
            }
            if (search) {
                // `like` compiles to PocketBase's `~`, whose wildcard is `%`, so
                // this is a contains-match across the three columns a human
                // would recognise an entry by. Chained `.where()` calls AND
                // together, which is why each predicate gets its own call rather
                // than being collected into one `and(...)`.
                q = q.where(({ audit_logs }) =>
                    or(
                        like(audit_logs.resource_label, `%${search}%`),
                        like(audit_logs.action, `%${search}%`),
                        like(audit_logs.resource_type, `%${search}%`)
                    )
                )
            }
            // Left-join the actor rather than reading it off `row.expand`: a
            // row's embedded copy is a shape the collection controls, so a
            // hand-written `expand?` keeps compiling after it goes away and the
            // name silently falls back to "System". `users` is on-demand, so the
            // join batch-loads the actors these rows name — one request by id for
            // the set, not one per row, and nothing for actors already held.
            return q
                .join({ actor: usersCollection }, ({ audit_logs, actor }) =>
                    eq(audit_logs.actor, actor.id)
                )
                .orderBy(({ audit_logs }) => audit_logs.created, 'desc')
                .limit(auditWindowSize(page))
                .select(({ audit_logs, actor }) => ({
                    ...audit_logs,
                    actorName: actor?.name,
                    actorEmail: actor?.email,
                }))
        },
    })

    const rows = logs ?? []

    if (rows.length === 0) {
        return (
            <Text className="text-muted-foreground text-sm mt-2">No audit log entries found.</Text>
        )
    }

    return (
        <View className="gap-3">
            <View className="rounded-xl border border-border overflow-hidden bg-surface-secondary">
                {rows.map(entry => (
                    <AuditLogRow key={entry.id} entry={entry} />
                ))}
            </View>
            <View className="flex-row items-center justify-between gap-3">
                <Text className="text-muted-foreground text-xs">Showing {rows.length}</Text>
                <LoadMoreButton
                    isVisible={hasMoreAuditPages(rows.length, page)}
                    onPress={() => setPage(p => p + 1)}
                />
            </View>
        </View>
    )
}

function LoadMoreButton({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    if (!isVisible) return null
    return (
        <Pressable
            onPress={onPress}
            testID="audit-load-more"
            className="px-3 py-1.5 rounded-md border border-border"
        >
            <Text className="text-primary text-xs font-semibold">Load {AUDIT_PAGE_SIZE} more</Text>
        </Pressable>
    )
}

interface AuditEntry {
    id: string
    action: AuditAction
    resource_type: string
    resource_id: string
    resource_label: string
    changes: Record<string, { before?: unknown; after?: unknown; redacted?: boolean }> | null
    snapshot: Record<string, unknown> | null
    created: string
    actorName?: string
    actorEmail?: string
}

function AuditLogRow({ entry }: { entry: AuditEntry }) {
    const [expanded, setExpanded] = useState(false)
    const mutedColor = useThemeColor('muted-foreground')

    const actorName = entry.actorName || entry.actorEmail || 'System'
    const hasDetails = Boolean(
        (entry.action === 'updated' && entry.changes && Object.keys(entry.changes).length > 0) ||
            (entry.action === 'deleted' && entry.snapshot && Object.keys(entry.snapshot).length > 0)
    )

    const resourceLabel = formatResourceType(entry.resource_type)

    return (
        <View className="border-b border-border">
            <Pressable
                className="px-4 py-3 flex-row items-center gap-3"
                onPress={() => hasDetails && setExpanded(!expanded)}
            >
                <View className="flex-1 gap-1">
                    <View className="flex-row gap-2 items-center flex-wrap">
                        <Text className="text-foreground text-sm font-semibold">{actorName}</Text>
                        <ActionBadge action={entry.action} />
                        <Text className="text-muted-foreground text-[13px]">{resourceLabel}</Text>
                    </View>
                    <EntryLabel isVisible={!!entry.resource_label} label={entry.resource_label} />
                    <Text className="text-muted-foreground text-xs">
                        {formatRelativeTime(entry.created)}
                    </Text>
                </View>
                <ExpandIcon isVisible={hasDetails} expanded={expanded} color={mutedColor} />
            </Pressable>

            <AuditDetails
                isVisible={expanded}
                action={entry.action}
                changes={entry.changes}
                snapshot={entry.snapshot}
            />
        </View>
    )
}

function EntryLabel({ isVisible, label }: { isVisible: boolean; label: string }) {
    if (!isVisible) return null
    return <Text className="text-foreground text-[13px]">{label}</Text>
}

function ExpandIcon({
    isVisible,
    expanded,
    color,
}: {
    isVisible: boolean
    expanded: boolean
    color: string
}) {
    if (!isVisible) return null
    return expanded ? (
        <ChevronUp size={16} color={color} />
    ) : (
        <ChevronDown size={16} color={color} />
    )
}

function ActionBadge({ action }: { action: string }) {
    const key = (action in ACTION_BADGE_CLASS ? action : 'updated') as AuditAction
    return (
        <View className={`rounded px-1.5 py-0.5 ${ACTION_BADGE_CLASS[key]}`}>
            <Text className={`text-[11px] font-semibold ${ACTION_TEXT_CLASS[key]}`}>{action}</Text>
        </View>
    )
}

function AuditDetails({
    isVisible,
    action,
    changes,
    snapshot,
}: {
    isVisible: boolean
    action: string
    changes: Record<string, { before?: unknown; after?: unknown; redacted?: boolean }> | null
    snapshot: Record<string, unknown> | null
}) {
    if (!isVisible) return null

    if (action === 'updated' && changes) {
        return (
            <View className="px-4 pb-3 gap-1.5">
                {Object.entries(changes).map(([field, change]) => (
                    <View key={field} className="rounded-md p-2 bg-surface-secondary">
                        <Text className="text-foreground text-xs font-semibold">{field}</Text>
                        <ChangeDetail isVisible={!change.redacted} change={change} />
                        <RedactedLabel isVisible={!!change.redacted} />
                    </View>
                ))}
            </View>
        )
    }

    if (action === 'deleted' && snapshot) {
        return (
            <View className="px-4 pb-3 gap-1.5">
                {Object.entries(snapshot).map(([field, value]) => (
                    <View key={field} className="flex-row gap-2">
                        <Text className="text-muted-foreground text-xs font-semibold">
                            {field}:
                        </Text>
                        <Text className="flex-1 text-foreground text-xs">{formatValue(value)}</Text>
                    </View>
                ))}
            </View>
        )
    }

    return null
}

function ChangeDetail({
    isVisible,
    change,
}: {
    isVisible: boolean
    change: { before?: unknown; after?: unknown }
}) {
    if (!isVisible) return null

    return (
        <View className="gap-0.5 mt-1">
            <Text className="text-danger text-[11px]">- {formatValue(change.before)}</Text>
            <Text className="text-success text-[11px]">+ {formatValue(change.after)}</Text>
        </View>
    )
}

function RedactedLabel({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return <Text className="text-muted-foreground text-[11px] italic">[redacted]</Text>
}

function formatResourceType(resourceType: string): string {
    return resourceType.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
}

function formatValue(val: unknown): string {
    if (val === null || val === undefined) return '(empty)'
    if (typeof val === 'object') return JSON.stringify(val)
    return String(val)
}

function formatRelativeTime(dateStr: string): string {
    const date = new Date(dateStr.replace(' ', 'T'))
    const now = new Date()
    const diffMs = now.getTime() - date.getTime()
    const diffMin = Math.floor(diffMs / 60000)

    if (diffMin < 1) return 'just now'
    if (diffMin < 60) return `${diffMin}m ago`
    const diffHr = Math.floor(diffMin / 60)
    if (diffHr < 24) return `${diffHr}h ago`
    const diffDay = Math.floor(diffHr / 24)
    if (diffDay < 30) return `${diffDay}d ago`
    return date.toLocaleDateString()
}
