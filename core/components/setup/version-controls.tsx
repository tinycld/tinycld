import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import { ArrowUp, ChevronDown, ChevronsUpDown } from 'lucide-react-native'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { compareVersions, type PackageVersionInfo } from './version-compare'

// Per-row version controls shared by the merged Packages screen: the version
// picker (RowVersionSelect) and the staged-direction flag (ChangeFlag). Split
// out so the row component stays readable and these stay reusable.

// Git tags come back `v`-prefixed (v1.0.0) while the registry's `current` is the
// bare manifest version (1.0.0), so a raw `===` never matches across the two.
// Compare by semver value instead (compareVersions tolerates a leading `v`).
export function sameVersion(a: string, b: string): boolean {
    return compareVersions(a, b) === 0
}

// Strip a leading `v` for display so a `v`-prefixed git tag doesn't render as
// `vv1.0.0` once the `v` prefix is re-added in the option label.
export function bareVersion(v: string): string {
    return v.replace(/^v/, '')
}

export function buildVersionOptions(info: PackageVersionInfo) {
    // available is typed string[] but the server can omit it (nil slice → JSON
    // null) for unknown-source packages; guard so a stale/edge response can't crash
    // the row on `.length`.
    const available = info.available ?? []
    const list = available.length > 0 ? available : info.current ? [info.current] : []
    // `value` keeps the raw discovered string (the git tag, e.g. `v1.0.0`) so it
    // round-trips to the install spec; `label` displays the bare version and marks
    // the current one via a semver-aware compare (tag vs bare registry version).
    return list.map(v => ({
        label: sameVersion(v, info.current) ? `v${bareVersion(v)} (current)` : `v${bareVersion(v)}`,
        value: v,
    }))
}

// 'up'/'down' are the staged direction (semver-aware, tolerant of a `v` prefix);
// 'none' = nothing staged (target equals current or absent).
export type RowDirection = 'up' | 'down' | 'none'

export function stagedDirection(info: PackageVersionInfo, target?: string): RowDirection {
    if (!target || sameVersion(target, info.current)) return 'none'
    const cmp = compareVersions(target, info.current)
    if (cmp === null) return 'down' // unknown ordering → treat as downgrade (matches hook)
    return cmp > 0 ? 'up' : 'down'
}

// RowVersionSelect is a standalone (non-RHF) version picker. SelectInput is
// react-hook-form-bound, so for a per-row select we use the anchored Menu
// primitive (correct positioning + a11y). A row that can't change (≤1 option)
// renders plain static text instead — the merged screen shows the bare current
// version there, with no dropdown affordance.
export function RowVersionSelect({
    options,
    value,
    direction,
    onChange,
}: {
    options: { label: string; value: string }[]
    value: string
    direction: RowDirection
    onChange: (v: string) => void
}) {
    const fgColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const successColor = useThemeColor('success')
    const warningColor = useThemeColor('warning')
    // `value` is the bare registry version (or a chosen raw-tag target); option
    // values are raw tags. Match by semver so the current/selected option resolves
    // its label (e.g. `v1.0.0 (current)`) instead of falling through to a bare
    // `v1.0.0` that loses the (current) marker.
    const selected = options.find(o => sameVersion(o.value, value))
    const label = selected?.label ?? `v${bareVersion(value)}`

    // Tint the trigger by staged direction so a glance down the column shows what
    // moves where, even before reading the change flag.
    const triggerBorder =
        direction === 'up' ? successColor : direction === 'down' ? warningColor : `${mutedColor}40`
    const triggerBg =
        direction === 'up'
            ? 'bg-success-soft'
            : direction === 'down'
              ? 'bg-warning-soft'
              : 'bg-surface'

    return (
        <Menu>
            <Menu.Trigger>
                <Pressable
                    className={`flex-row items-center justify-between gap-2 px-3 py-2 rounded-lg border ${triggerBg}`}
                    style={{ borderColor: triggerBorder }}
                >
                    <Text
                        style={{ color: fgColor, fontFamily: 'monospace', fontSize: 13 }}
                        numberOfLines={1}
                    >
                        {label}
                    </Text>
                    <ChevronsUpDown size={14} color={mutedColor} />
                </Pressable>
            </Menu.Trigger>
            <Menu.Portal>
                <Menu.Overlay />
                <Menu.Content align="end" className="max-h-[360px]">
                    <ScrollView>
                        {options.map(o => (
                            <Menu.Item
                                key={o.value}
                                onPress={() => onChange(o.value)}
                                className={o.value === value ? 'bg-accent/20' : ''}
                            >
                                <Menu.ItemTitle>{o.label}</Menu.ItemTitle>
                            </Menu.Item>
                        ))}
                    </ScrollView>
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}

export function ChangeFlag({ direction }: { direction: RowDirection }) {
    const successColor = useThemeColor('success')
    const warningColor = useThemeColor('warning')

    if (direction === 'up') {
        return (
            <View className="flex-row items-center gap-1.5">
                <ArrowUp size={14} color={successColor} />
                <Text style={{ color: successColor, fontSize: 12, fontWeight: '600' }}>
                    upgrade
                </Text>
            </View>
        )
    }
    if (direction === 'down') {
        return (
            <View className="flex-row items-center gap-1.5">
                <ChevronDown size={14} color={warningColor} />
                <Text style={{ color: warningColor, fontSize: 12, fontWeight: '600' }}>
                    downgrade
                </Text>
            </View>
        )
    }
    return null
}
