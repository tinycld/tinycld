import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { GripVertical } from 'lucide-react-native'
import type { ReactNode } from 'react'
import { useRef } from 'react'
import { ScrollView, View } from 'react-native'
import { DraxHandle, SortableContainer, SortableItem, useSortableList } from 'react-native-drax'

interface SortableListProps<T> {
    data: T[]
    keyExtractor: (item: T, index: number) => string
    /** Committed on drop with the reordered array. */
    onReorder: (data: T[]) => void
    /**
     * Renders a row. Place a <SortableDragHandle/> somewhere in the row to
     * start a drag — activation is handle-only, so interactive controls
     * elsewhere in the row stay tappable. While a row is being dragged Drax
     * hides it and shows a floating hover copy, so the row itself never needs
     * to render an "active" state of its own.
     */
    renderItem: (params: { item: T; index: number }) => ReactNode
    /** Rows for which this returns false stay pinned (cannot be dragged). */
    isDraggable?: (item: T, index: number) => boolean
}

/**
 * Handle-activated, single-list reorder built on react-native-drax's sortable
 * primitives. Replaces react-native-draggable-flatlist for the app's short,
 * non-scrolling reorder editors (package order, nav order). The list never
 * scrolls itself — it sizes to its content inside a scrollEnabled={false}
 * ScrollView, matching the previous behaviour.
 */
export function SortableList<T>({
    data,
    keyExtractor,
    onReorder,
    renderItem,
    isDraggable,
}: SortableListProps<T>) {
    const scrollRef = useRef<ScrollView>(null)
    const sortable = useSortableList<T>({
        data,
        keyExtractor,
        onReorder: event => onReorder(event.data),
        longPressDelay: 1,
    })

    return (
        <SortableContainer sortable={sortable} scrollRef={scrollRef}>
            <ScrollView ref={scrollRef} scrollEnabled={false}>
                {sortable.data.map((item, index) => (
                    <SortableItem
                        key={sortable.stableKeyExtractor(item, index)}
                        sortable={sortable}
                        index={index}
                        dragHandle
                        fixed={isDraggable ? !isDraggable(item, index) : false}
                    >
                        {renderItem({ item, index })}
                    </SortableItem>
                ))}
            </ScrollView>
        </SortableContainer>
    )
}

interface SortableDragHandleProps {
    disabled?: boolean
    color?: string
    size?: number
}

/**
 * The grip that starts a drag inside a SortableList row. Must be rendered
 * within a row produced by SortableList's renderItem. When disabled it renders
 * a greyed-out grip and does not attach the drag gesture.
 */
export function SortableDragHandle({ disabled, color, size = 18 }: SortableDragHandleProps) {
    const mutedColor = useThemeColor('muted-foreground')
    const iconColor = color ?? mutedColor

    if (disabled) {
        return (
            <View className="pr-3" style={{ opacity: 0.4 }}>
                <GripVertical size={size} color={iconColor} />
            </View>
        )
    }

    return (
        <DraxHandle style={{ paddingRight: 12 }}>
            <GripVertical size={size} color={iconColor} />
        </DraxHandle>
    )
}
