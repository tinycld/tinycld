import { FlashList, type FlashListRef } from '@shopify/flash-list'
import { parseNativeEmoji } from '@tinycld/core/lib/emoji/parse'
import { applyTone, type ToneChoice } from '@tinycld/core/lib/emoji/tones'
import { type RefObject, useCallback } from 'react'
import { Pressable, Text, View } from 'react-native'
import { CATEGORY_LABELS } from './categories'
import type { EmojiRecord } from './emoji-data'
import { EMOJI_SIZE, GRID_HEIGHT, ROW_HEIGHT, SECTION_HEADER_HEIGHT } from './layout'
import type { EmojiRow } from './rows'

export interface EmojiPickerGridProps {
    rows: readonly EmojiRow[]
    /** Held by the host so the category nav can scroll to a section. */
    listRef?: RefObject<FlashListRef<EmojiRow> | null>
    /** Reports the topmost visible section, to light the nav. */
    onVisibleCategoryChange?: (category: string) => void
    /** The tone applied to any emoji that takes one. */
    tone: ToneChoice
    onSelect: (glyph: string) => void
    /** Shown in place of the grid while the lazy table is in flight. */
    isLoading?: boolean
    emptyLabel?: string
}

/**
 * The scrolling emoji grid. Presentational — the host owns the popover, the
 * search field and the category nav.
 *
 * Fixed height on purpose: Popover derives its own maxHeight by measuring
 * scroll content, and a virtualized child reports a size that fights that.
 */
export function EmojiPickerGrid({
    rows,
    listRef,
    onVisibleCategoryChange,
    tone,
    onSelect,
    isLoading = false,
    emptyLabel = 'No emoji found',
}: EmojiPickerGridProps) {
    const renderItem = useCallback(
        ({ item }: { item: EmojiRow }) =>
            item.kind === 'header' ? (
                <SectionHeader category={item.category} />
            ) : (
                <EmojiRowCells emoji={item.emoji} tone={tone} onSelect={onSelect} />
            ),
        [tone, onSelect]
    )

    // The topmost visible row's section, so the nav highlights what you are
    // looking at rather than only what you last tapped.
    const handleViewableItemsChanged = useCallback(
        ({ viewableItems }: { viewableItems: { item?: EmojiRow }[] }) => {
            if (!onVisibleCategoryChange) return
            const first = viewableItems[0]?.item
            if (!first) return
            const category = first.kind === 'header' ? first.category : first.key.split(':')[0]
            if (category) onVisibleCategoryChange(category)
        },
        [onVisibleCategoryChange]
    )

    if (isLoading) return <GridMessage text="Loading emoji…" />
    if (rows.length === 0) return <GridMessage text={emptyLabel} />

    return (
        <View style={{ height: GRID_HEIGHT }}>
            <FlashList<EmojiRow>
                ref={listRef}
                data={rows as EmojiRow[]}
                renderItem={renderItem}
                keyExtractor={row => row.key}
                getItemType={row => row.kind}
                onViewableItemsChanged={handleViewableItemsChanged}
                showsVerticalScrollIndicator={false}
            />
        </View>
    )
}

function GridMessage({ text }: { text: string }) {
    return (
        <View style={{ height: GRID_HEIGHT }} className="items-center justify-center">
            <Text className="text-[13px] text-muted">{text}</Text>
        </View>
    )
}

function SectionHeader({ category }: { category: string }) {
    return (
        <View style={{ height: SECTION_HEADER_HEIGHT }} className="justify-end px-1 pb-1">
            <Text className="text-[11px] font-medium uppercase text-muted">
                {CATEGORY_LABELS[category] ?? category}
            </Text>
        </View>
    )
}

function EmojiRowCells({
    emoji,
    tone,
    onSelect,
}: {
    emoji: readonly EmojiRecord[]
    tone: ToneChoice
    onSelect: (glyph: string) => void
}) {
    return (
        <View className="flex-row" style={{ height: ROW_HEIGHT }}>
            {emoji.map(record => (
                <EmojiCell key={record.u} record={record} tone={tone} onSelect={onSelect} />
            ))}
        </View>
    )
}

function EmojiCell({
    record,
    tone,
    onSelect,
}: {
    record: EmojiRecord
    tone: ToneChoice
    onSelect: (glyph: string) => void
}) {
    // A tone applies only where the table says the emoji takes one; applying
    // it elsewhere would build a sequence no font can render.
    const unified = record.t ? applyTone(record.u, tone) : record.u
    const glyph = parseNativeEmoji(unified)
    const name = record.n[record.n.length - 1]

    return (
        <Pressable
            onPress={() => onSelect(glyph)}
            accessibilityRole="button"
            accessibilityLabel={name}
            testID={`emoji-pick-${record.u}`}
            style={{ width: EMOJI_SIZE, height: EMOJI_SIZE }}
            className="items-center justify-center rounded hover:bg-accent web:outline-none web:focus-visible:ring-2 web:focus-visible:ring-ring"
        >
            <Text className="text-[22px]">{glyph}</Text>
        </Pressable>
    )
}
