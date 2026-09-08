import type { FlashListRef } from '@shopify/flash-list'
import { Popover } from '@tinycld/core/ui/popover'
import { type ReactElement, useCallback, useRef, useState } from 'react'
import { View } from 'react-native'
import { CategoryNav } from './CategoryNav'
import { CATEGORY_ORDER } from './categories'
import { EmojiPickerGrid } from './EmojiPickerGrid'
import { EmojiSearch } from './EmojiSearch'
import { PICKER_WIDTH } from './layout'
import type { EmojiRow } from './rows'
import { useEmojiData } from './use-emoji-data'
import { useEmojiPicker } from './use-emoji-picker'

export interface EmojiPickerProps {
    /**
     * Opens the picker. Cloned by Popover to inject onPress and a measured
     * ref, so it must forward both — see the warning in components/Tooltip.tsx
     * about wrapping a trigger in anything that swallows them.
     */
    trigger: ReactElement
    onPick: (glyph: string) => void
    placement?: 'bottom-start' | 'bottom-end' | 'top-start' | 'top-end'
    testID?: string
}

/**
 * The full emoji picker: search, categories, skin tones and frequently-used.
 *
 * The table it renders is ~99KB and loads only when the picker first opens
 * (use-emoji-data.ts). Until then this component costs a trigger and a closed
 * Popover.
 */
export function EmojiPicker({
    trigger,
    onPick,
    placement = 'bottom-start',
    testID,
}: EmojiPickerProps) {
    const [isOpen, setIsOpen] = useState(false)

    return (
        <Popover
            isOpen={isOpen}
            onOpenChange={setIsOpen}
            trigger={trigger}
            placement={placement}
            width={PICKER_WIDTH}
            title="Add reaction"
            testID={testID}
        >
            <PickerBody
                isOpen={isOpen}
                onPick={glyph => {
                    onPick(glyph)
                    setIsOpen(false)
                }}
            />
        </Popover>
    )
}

/**
 * Split from EmojiPicker so the table loads on OPEN rather than on mount: the
 * body only exists while the surface is up, and `isOpen` gates the fetch.
 */
function PickerBody({ isOpen, onPick }: { isOpen: boolean; onPick: (glyph: string) => void }) {
    const { table, isLoading } = useEmojiData(isOpen)
    const { query, setQuery, tone, setTone, rows, isSearching, recordUse } = useEmojiPicker(table)
    const listRef = useRef<FlashListRef<EmojiRow> | null>(null)
    const [visibleCategory, setVisibleCategory] = useState<string | null>(null)

    const pick = (glyph: string) => {
        recordUse(glyph)
        onPick(glyph)
    }

    const jumpToCategory = useCallback(
        (category: string) => {
            const index = rows.findIndex(row => row.kind === 'header' && row.category === category)
            if (index >= 0) listRef.current?.scrollToIndex({ index, animated: false })
        },
        [rows]
    )

    // Only categories with emoji get a nav button — "frequently used" has none
    // until someone has used something, and a dead tab is worse than no tab.
    const presentCategories = CATEGORY_ORDER.filter(category =>
        rows.some(row => row.kind === 'header' && row.category === category)
    )

    return (
        <View>
            <EmojiSearch
                query={query}
                onQueryChange={setQuery}
                tone={tone}
                onToneChange={setTone}
            />
            <CategoryNav
                categories={presentCategories}
                active={isSearching ? null : visibleCategory}
                onSelect={jumpToCategory}
            />
            <EmojiPickerGrid
                rows={rows}
                listRef={listRef}
                onVisibleCategoryChange={setVisibleCategory}
                tone={tone}
                onSelect={pick}
                isLoading={isLoading}
                emptyLabel={isSearching ? 'No emoji found' : 'No emoji'}
            />
        </View>
    )
}
