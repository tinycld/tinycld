import { useQuery } from '@tanstack/react-query'
import { getIcon } from '@tinycld/core/components/workspace/package-icon-map'
import type { SearchSection } from '@tinycld/core/lib/search/build-sections'
import { chipsToText, runHandlerFor } from '@tinycld/core/lib/search/chip-text'
import { parseQuery } from '@tinycld/core/lib/search/parse-query'
import { loadSearchAdapter, searchPackages } from '@tinycld/core/lib/search/registry'
import { useSearchPaletteStore } from '@tinycld/core/lib/search/search-palette-store'
import type { SearchAdapterModule, SearchRow } from '@tinycld/core/lib/search/types'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { type ReactNode, type RefObject, useCallback, useEffect, useMemo, useRef } from 'react'
import { createPortal } from 'react-dom'
import { Pressable, Text, TextInput, View } from 'react-native'
import { useSearchResults } from './useSearchResults'

const PALETTE_WIDTH_PX = 560
const PALETTE_TOP_OFFSET_PX = 96

// Inject the open-animation keyframes once at module load — distinct id from
// the help palette's <style> tag so the two injections cannot collide. React
// Native's StyleSheet doesn't expose @keyframes, so a tiny static <style> tag
// is simpler than a JS-driven Animated value for an animation that runs once
// on mount and never again. Guarded for SSR.
if (typeof document !== 'undefined' && !document.getElementById('tinycld-search-palette-styles')) {
    const styleEl = document.createElement('style')
    styleEl.id = 'tinycld-search-palette-styles'
    styleEl.textContent = `@keyframes tinycld-search-palette-in {
        from { opacity: 0; transform: translateY(-4px); }
        to { opacity: 1; transform: translateY(0); }
    }`
    document.head.appendChild(styleEl)
}

const INSTALLED_SLUGS = searchPackages.map(p => p.slug)

// SearchPalette renders the global cross-package command palette. Listens to
// the Zustand store driven by the `/` shortcut. Web-only: portals to
// document.body so the overlay sits above any package's own scroll
// container.
export function SearchPalette() {
    const isOpen = useSearchPaletteStore(s => s.isOpen)
    const text = useSearchPaletteStore(s => s.text)
    const selectedRowId = useSearchPaletteStore(s => s.selectedRowId)
    const setText = useSearchPaletteStore(s => s.setText)
    const setSelectedRowId = useSearchPaletteStore(s => s.setSelectedRowId)
    const close = useSearchPaletteStore(s => s.close)

    const parsed = useMemo(() => parseQuery(text, INSTALLED_SLUGS), [text])
    const { sections, partial } = useSearchResults(parsed)

    const flatRows = useMemo(() => sections.flatMap(s => s.rows), [sections])
    const selectedRow = flatRows.find(r => r.id === selectedRowId) ?? flatRows[0]

    const handlersRef = useRef<Record<string, (row: SearchRow) => void>>({})
    const registerHandler = useCallback((slug: string, handler: (row: SearchRow) => void) => {
        handlersRef.current[slug] = handler
    }, [])

    const selectRow = useCallback(
        (row: SearchRow) => {
            // Only close when a handler actually ran — see runHandlerFor's
            // doc comment for why a row can outlive its own adapter.
            if (runHandlerFor(row, handlersRef.current)) close()
        },
        [close]
    )

    const moveSelection = useCallback(
        (delta: number) => {
            if (flatRows.length === 0) return
            const current = flatRows.findIndex(r => r.id === selectedRow?.id)
            const next = (current + delta + flatRows.length) % flatRows.length
            setSelectedRowId(flatRows[next].id)
        },
        [flatRows, selectedRow, setSelectedRowId]
    )

    const inputRef = useRef<TextInput>(null)

    useEffect(() => {
        if (!isOpen) return
        // Focus the input on open. RN's TextInput needs a microtask before its
        // DOM node is mounted under the portal, so we defer via
        // requestAnimationFrame.
        const raf = requestAnimationFrame(() => {
            inputRef.current?.focus()
        })
        return () => cancelAnimationFrame(raf)
    }, [isOpen])

    // Click-outside dismiss. Anything outside the palette card closes it —
    // matches Spotlight + the help palette's pattern.
    useEffect(() => {
        if (!isOpen || typeof document === 'undefined') return
        function onPointerDown(event: MouseEvent) {
            const target = event.target
            if (!(target instanceof Element)) return
            if (target.closest('[data-tinycld-search-palette]')) return
            close()
        }
        document.addEventListener('mousedown', onPointerDown, true)
        return () => document.removeEventListener('mousedown', onPointerDown, true)
    }, [isOpen, close])

    // Keyboard navigation. Bound to document keydown in CAPTURE phase while
    // the palette is open. Capture phase matters because react-native-web's
    // TextInput swallows Escape on its own internal bubble-phase handler — a
    // non-capturing listener never sees it. ArrowUp/ArrowDown likewise need
    // this because TextInput.onKeyPress doesn't fire for non-character keys.
    useEffect(() => {
        if (!isOpen || typeof document === 'undefined') return
        function onKeyDown(event: KeyboardEvent) {
            const key = event.key
            if (key === 'Escape') {
                event.preventDefault()
                close()
                return
            }
            if (key === 'ArrowDown') {
                event.preventDefault()
                moveSelection(1)
                return
            }
            if (key === 'ArrowUp') {
                event.preventDefault()
                moveSelection(-1)
                return
            }
            if (key === 'Enter') {
                event.preventDefault()
                if (selectedRow) selectRow(selectedRow)
                return
            }
            // Backspace on empty text pops the trailing chip, so widening the
            // search to everywhere is one keystroke from the seeded state.
            if (key === 'Backspace') {
                if (parsed.remainder.length === 0 && parsed.chips.length > 0) {
                    event.preventDefault()
                    setText(chipsToText(parsed.chips.slice(0, -1)))
                }
            }
        }
        document.addEventListener('keydown', onKeyDown, true)
        return () => document.removeEventListener('keydown', onKeyDown, true)
    }, [
        isOpen,
        close,
        parsed.chips,
        parsed.remainder,
        moveSelection,
        selectedRow,
        selectRow,
        setText,
    ])

    if (!isOpen || typeof document === 'undefined') return null

    const remainder = parsed.remainder

    return createPortal(
        <>
            {searchPackages.map(pkg => (
                <PackageActions key={pkg.slug} slug={pkg.slug} onReady={registerHandler} />
            ))}
            <PaletteOverlay>
                <PaletteCard>
                    <SearchField
                        inputRef={inputRef}
                        chips={parsed.chips}
                        remainder={remainder}
                        onChangeRemainder={next => setText(chipsToText(parsed.chips) + next)}
                    />
                    <ResultList
                        sections={sections}
                        selectedRowId={selectedRow?.id ?? null}
                        onPick={selectRow}
                        onHover={id => setSelectedRowId(id)}
                    />
                    <FooterHints chips={parsed.chips} remainder={remainder} partial={partial} />
                </PaletteCard>
            </PaletteOverlay>
        </>,
        document.body
    )
}

function useAdapterModule(slug: string): SearchAdapterModule | null {
    const { data } = useQuery({
        queryKey: ['search-adapter', slug],
        queryFn: () => loadSearchAdapter(slug),
        staleTime: Number.POSITIVE_INFINITY,
    })
    return (data as SearchAdapterModule | null) ?? null
}

interface PackageActionsProps {
    slug: string
    onReady: (slug: string, handler: (row: SearchRow) => void) => void
}

/**
 * Loads one package's adapter module. A component per package rather than a
 * loop of hook calls inside the palette: the adapter's `useSearchActions` is
 * a hook, and calling it in a loop would break the Rules of Hooks the moment
 * the package list changed.
 *
 * This component itself renders nothing until the adapter has resolved, and
 * deliberately does NOT call `adapter?.useSearchActions()` here — that would
 * call a hook conditionally (skipped while the dynamic import is pending,
 * called once it lands), which is exactly the "rendered more hooks than
 * during the previous render" crash the Rules of Hooks forbid. Instead the
 * hook call is pushed into `ResolvedPackageActions`, a component that only
 * ever mounts once `adapter` is non-null — so for that component's entire
 * lifetime the hook call is unconditional.
 */
export function PackageActions({ slug, onReady }: PackageActionsProps) {
    const adapter = useAdapterModule(slug)
    if (!adapter) return null
    return <ResolvedPackageActions slug={slug} adapter={adapter} onReady={onReady} />
}

/**
 * Registers one resolved adapter's selection handler. Split out from
 * `PackageActions` so `adapter` is non-null for this component's entire
 * lifetime, making the `useSearchActions()` call unconditional.
 */
function ResolvedPackageActions({
    slug,
    adapter,
    onReady,
}: PackageActionsProps & { adapter: SearchAdapterModule }) {
    const { onSelect } = adapter.useSearchActions()
    // Registering during render would mutate a parent ref mid-render; a ref
    // write in an effect is the standard imperative-handle pattern.
    useEffect(() => {
        onReady(slug, onSelect)
    }, [slug, onSelect, onReady])
    return null
}

function PaletteOverlay({ children }: { children: ReactNode }) {
    // Full-viewport fixed layer so the click-outside listener has somewhere
    // to fire. The card itself is positioned within.
    return (
        <View
            style={
                {
                    position: 'fixed',
                    top: 0,
                    left: 0,
                    right: 0,
                    bottom: 0,
                    alignItems: 'center',
                    paddingTop: PALETTE_TOP_OFFSET_PX,
                    zIndex: 1000,
                } as object
            }
            pointerEvents="box-none"
        >
            {children}
        </View>
    )
}

function PaletteCard({ children }: { children: ReactNode }) {
    // animationName drives the open animation defined in the module-level
    // <style> tag. RN ignores CSS animation properties on native, but this
    // component is .web.tsx so we're safe to use browser semantics directly.
    const animationStyle = {
        animationName: 'tinycld-search-palette-in',
        animationDuration: '120ms',
        animationTimingFunction: 'ease-out',
    } as object
    const cardDomProps = {
        'data-tinycld-search-palette': 'true',
        role: 'dialog',
        'aria-label': 'Search',
    } as object
    return (
        <View
            {...(cardDomProps as Record<string, unknown>)}
            style={
                {
                    width: PALETTE_WIDTH_PX,
                    maxWidth: '90%',
                    maxHeight: '60vh',
                    ...animationStyle,
                } as object
            }
            className="rounded-xl border border-border bg-background shadow-lg overflow-hidden"
        >
            {children}
        </View>
    )
}

interface SearchFieldProps {
    inputRef: RefObject<TextInput | null>
    chips: string[]
    remainder: string
    onChangeRemainder: (v: string) => void
}

// Scope chips render as their own row of pills — separate from the text
// input — so a test (and a user) can see and target "boards" as a discrete
// unit rather than parsing it back out of a raw "boards: " string prefix.
// The input itself only ever holds the free-text remainder; typing composes
// chips + remainder back into the store's single `text` field, which stays
// the source of truth parseQuery re-derives BOTH chips and remainder from on
// every render — the renderer never re-slices the remainder itself.
function SearchField({ inputRef, chips, remainder, onChangeRemainder }: SearchFieldProps) {
    const placeholderColor = useThemeColor('muted-foreground')
    return (
        <View className="px-4 pt-4 pb-2 flex-row items-center flex-wrap gap-1.5">
            {chips.map(slug => (
                <SearchChip key={slug} slug={slug} />
            ))}
            <TextInput
                ref={inputRef}
                value={remainder}
                onChangeText={onChangeRemainder}
                placeholder={chips.length === 0 ? 'Search everywhere…' : 'Search…'}
                placeholderTextColor={placeholderColor}
                className="text-base text-foreground flex-1 min-w-[80px]"
                style={{ outlineWidth: 0 } as object}
                accessibilityLabel="Search across packages"
            />
        </View>
    )
}

function SearchChip({ slug }: { slug: string }) {
    const pkg = searchPackages.find(p => p.slug === slug)
    const chipDomProps = { 'data-testid': `search-chip-${slug}` } as Record<string, unknown>
    return (
        <View
            {...chipDomProps}
            className="rounded-md bg-surface-secondary px-2 py-1 flex-row items-center"
        >
            <Text className="text-xs font-medium text-foreground">{pkg?.label ?? slug}</Text>
        </View>
    )
}

interface ResultListProps {
    sections: SearchSection[]
    selectedRowId: string | null
    onPick: (row: SearchRow) => void
    onHover: (id: string) => void
}

function ResultList({ sections, selectedRowId, onPick, onHover }: ResultListProps) {
    if (sections.length === 0 || sections.every(s => s.rows.length === 0)) {
        return <EmptyState />
    }
    return (
        <View style={{ maxHeight: '40vh', overflowY: 'auto' } as object}>
            {sections.map((section, index) => (
                <SectionBlock
                    key={section.title ?? `flat-${index}`}
                    section={section}
                    selectedRowId={selectedRowId}
                    onPick={onPick}
                    onHover={onHover}
                />
            ))}
        </View>
    )
}

interface SectionBlockProps {
    section: SearchSection
    selectedRowId: string | null
    onPick: (row: SearchRow) => void
    onHover: (id: string) => void
}

function SectionBlock({ section, selectedRowId, onPick, onHover }: SectionBlockProps) {
    return (
        <View>
            {section.title && <SectionHeading title={section.title} icon={section.icon} />}
            {section.rows.map(row => (
                <ResultRow
                    key={`${row.slug}:${row.id}`}
                    row={row}
                    showBadge={section.showBadges}
                    isSelected={row.id === selectedRowId}
                    onPress={() => onPick(row)}
                    onHoverIn={() => onHover(row.id)}
                />
            ))}
        </View>
    )
}

function SectionHeading({ title, icon }: { title: string; icon?: string }) {
    const Icon = getIcon(icon ?? '')
    const iconColor = useThemeColor('muted-foreground')
    const headingDomProps = { 'data-testid': 'search-section-heading' } as Record<string, unknown>
    return (
        <View {...headingDomProps} className="flex-row items-center gap-2 px-4 pt-3 pb-1">
            <Icon size={14} color={iconColor} />
            <Text className="text-xs font-medium text-muted-foreground uppercase">{title}</Text>
        </View>
    )
}

interface ResultRowProps {
    row: SearchRow
    showBadge: boolean
    isSelected: boolean
    onPress: () => void
    onHoverIn: () => void
}

function ResultRow({ row, showBadge, isSelected, onPress, onHoverIn }: ResultRowProps) {
    const pkg = useMemo(() => searchPackages.find(p => p.slug === row.slug), [row.slug])
    const optionDomProps = {
        role: 'option',
        'aria-selected': isSelected,
    } as const
    return (
        <Pressable
            onPress={onPress}
            onHoverIn={onHoverIn}
            {...optionDomProps}
            className={`px-4 py-3 flex-row items-center justify-between gap-3 ${
                isSelected ? 'bg-surface-secondary' : 'bg-transparent'
            }`}
        >
            <View className="flex-1">
                <Text className="text-base font-medium text-foreground" numberOfLines={1}>
                    {row.title}
                </Text>
                {row.subtitle && (
                    <Text className="text-xs text-muted-foreground mt-0.5" numberOfLines={1}>
                        {row.subtitle}
                    </Text>
                )}
            </View>
            {row.meta && (
                <Text className="text-xs text-muted-foreground" numberOfLines={1}>
                    {row.meta}
                </Text>
            )}
            {showBadge && pkg && (
                <Text className="text-xs text-muted-foreground px-2 py-0.5 rounded bg-surface-secondary">
                    {pkg.label}
                </Text>
            )}
        </Pressable>
    )
}

function EmptyState() {
    return (
        <View className="px-4 py-8 items-center">
            <Text className="text-sm text-muted-foreground">No results.</Text>
        </View>
    )
}

interface FooterHintsProps {
    chips: string[]
    remainder: string
    partial: string[]
}

// The footer is where the `:` and ⌫ grammar is discoverable, so it reacts to
// what the user has typed rather than showing three static hints.
function FooterHints({ chips, remainder, partial }: FooterHintsProps) {
    // A package the server could not reach takes priority over any grammar
    // hint: results are missing, and silence would read as "nothing there"
    // rather than "we could not look".
    if (partial.length > 0) {
        const labels = partial.map(slug => searchPackages.find(p => p.slug === slug)?.label ?? slug)
        return (
            <View className="px-4 py-2 border-t border-border">
                <Text className="text-xs text-muted-foreground">
                    {`Could not search ${labels.join(', ')} — results may be incomplete`}
                </Text>
            </View>
        )
    }

    // The last word matches a package but has no colon yet — offer the gesture.
    const lastWord = remainder.trim().split(/\s+/).pop() ?? ''
    const pending = searchPackages.find(
        p => p.slug === lastWord.toLowerCase() && !chips.includes(p.slug)
    )
    if (pending) {
        return <Hints items={['↑↓ move', '↵ open', `: scope to ${pending.slug}`, 'esc close']} />
    }
    if (chips.length > 0 && remainder.length === 0) {
        return (
            <Hints
                items={['↑↓ move', '↵ open', `⌫ remove ${chips[chips.length - 1]}`, 'esc close']}
            />
        )
    }
    return <Hints items={['↑↓ move', '↵ open', 'esc close']} />
}

function Hints({ items }: { items: string[] }) {
    return (
        <View className="flex-row gap-4 px-4 py-2 border-t border-border">
            {items.map(item => (
                <Text key={item} className="text-xs text-muted-foreground">
                    {item}
                </Text>
            ))}
        </View>
    )
}
