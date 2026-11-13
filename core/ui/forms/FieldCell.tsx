import type { FieldWidth } from '@tinycld/core/lib/forms/definition'
import type { ReactNode } from 'react'
import { Pressable, Text, View } from 'react-native'
import { FIELD_CELL_BORDER_WIDTH } from './fields/table-layout'
import { type LayoutRect, type RegisterMeasure, useMeasuredRect } from './use-measure-rect'
import { effectiveWidth, widthClass } from './widths'

export interface FieldCellProps {
    fieldKey: string
    label: string
    description?: string
    required: boolean
    error?: string
    width: FieldWidth
    /** The width below NARROW_BELOW; absent keeps the default mapping (see `effectiveWidth`). */
    narrowWidth?: FieldWidth
    containerWidth: number
    children: ReactNode
    testID?: string
    /** The builder's drag handle, rendered before the label. */
    handle?: ReactNode
    isSelected?: boolean
    /** The input shows the label itself (a toggle, a radio or choice list), so the cell does not. */
    hideLabel?: boolean
    /**
     * The field draws all of its own chrome: the cell is only the width
     * wrapper (basis and the side padding FormLayout's negative margin
     * cancels) — no label, description, required mark, border, error or
     * bottom margin. A field that draws nothing then takes no height.
     */
    isBare?: boolean
    /** The builder's per-cell marker (e.g. a rule badge), right of the label. */
    badge?: ReactNode
    /** Which edge shows the drop bar while a field is dragged over this cell. */
    dropIndicator?: 'before' | 'after'
    /** The builder is dragging this cell: it dims so the landing spot stands out. */
    isDragging?: boolean
    onLayoutRect?: (rect: LayoutRect) => void
    /** Hands the builder a thunk that re-measures this cell on demand. */
    registerMeasure?: RegisterMeasure
    /**
     * The builder selects the cell on press. The cell itself is then the
     * pressable, so it stays the row's flex item and keeps its width basis.
     */
    onPress?: () => void
}

// The border's color on both platforms; its width (`BOX_BORDER_STYLE`) is always present so selecting a cell never shifts its layout.
const SELECTED_CLASS = 'border-primary'
const UNSELECTED_CLASS = 'border-transparent'
const DROP_TARGET_CLASS = 'bg-primary/10'

/**
 * The visible box sits inside the root's side padding. The root reaches into
 * the layout's negative margin, past the content column, where a scroll view
 * clips; the box's edges stay inside the column, so its outline and drop bars
 * are never cut off.
 */
function boxClassFor(isSelected: boolean, isDropTarget: boolean, isSelectable: boolean) {
    const border = isSelected ? SELECTED_CLASS : UNSELECTED_CLASS
    const tint = isDropTarget ? DROP_TARGET_CLASS : ''
    // Room inside the outline, always there, so the outline never touches the content and selecting never shifts it.
    const room = isSelectable ? 'p-2' : ''
    return `relative rounded-md ${border} ${room} ${tint}`
}

// The width from the one exported size; the color class above only tints it.
const BOX_BORDER_STYLE = { borderWidth: FIELD_CELL_BORDER_WIDTH }

/**
 * The basis plus min-w-0: on web a flex item grows to its content's
 * min-content width, so an input's intrinsic width would widen the cell past
 * its basis. A fit-content cell is meant to be its content's width, so it
 * keeps that floor (its class caps it at the row instead).
 */
function sizingClassFor(width: FieldWidth) {
    const basis = widthClass(width)
    if (width === 'auto') return basis
    return `${basis} min-w-0`
}

/** The root is the row's flex item and the measured hit area, gaps included. */
function rootClassFor(sizing: string, isDragging: boolean) {
    const dim = isDragging ? 'opacity-50' : ''
    return `relative ${sizing} grow-0 shrink-0 px-1.5 mb-3 ${dim}`
}

function RequiredMark({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return <Text className="text-destructive ml-0.5">*</Text>
}

function CellLabel({
    fieldKey,
    label,
    isVisible,
}: {
    fieldKey: string
    label: string
    isVisible: boolean
}) {
    if (!isVisible) return null
    return (
        <Text
            className="shrink min-w-0 text-foreground text-sm font-medium"
            nativeID={`form-label-${fieldKey}`}
        >
            {label}
        </Text>
    )
}

// A full-width cell's neighbours sit above and below it, so its bars lie along its top and bottom edges.
const BAR_EDGES = {
    before: { row: 'top-0 bottom-0 left-0 w-[3px]', column: 'left-0 right-0 top-0 h-[3px]' },
    after: { row: 'top-0 bottom-0 right-0 w-[3px]', column: 'left-0 right-0 bottom-0 h-[3px]' },
}

interface DropBarProps {
    fieldKey: string
    side?: 'before' | 'after'
    isFullWidth: boolean
}

function DropBar({ fieldKey, side, isFullWidth }: DropBarProps) {
    if (!side) return null
    const edge = BAR_EDGES[side][isFullWidth ? 'column' : 'row']
    return (
        <View
            className={`absolute rounded bg-primary ${edge}`}
            testID={`form-field-${fieldKey}-drop-${side}`}
        />
    )
}

function CellError({ message, testID }: { message?: string; testID: string }) {
    if (!message) return null
    return (
        <Text className="text-destructive text-xs mt-1" accessibilityRole="alert" testID={testID}>
            {message}
        </Text>
    )
}

// Under the input, so the inputs of a row line up whether or not a field has a hint.
function CellDescription({ text }: { text?: string }) {
    if (!text) return null
    return <Text className="text-muted-foreground text-xs mt-1">{text}</Text>
}

export function FieldCell({
    fieldKey,
    label,
    description,
    required,
    error,
    width,
    narrowWidth,
    containerWidth,
    children,
    testID,
    handle,
    isSelected = false,
    hideLabel = false,
    isBare = false,
    badge,
    dropIndicator,
    isDragging = false,
    onLayoutRect,
    registerMeasure,
    onPress,
}: FieldCellProps) {
    const effective = effectiveWidth(width, containerWidth, narrowWidth)
    const sizing = sizingClassFor(effective)
    const { ref, onLayout } = useMeasuredRect(onLayoutRect, registerMeasure)
    const cellTestID = testID ?? `form-field-${fieldKey}`
    if (isBare) {
        return (
            <View
                ref={ref}
                onLayout={onLayout}
                className={`${sizing} grow-0 shrink-0 px-1.5`}
                testID={cellTestID}
            >
                {children}
            </View>
        )
    }
    const root = {
        ref,
        onLayout,
        className: rootClassFor(sizing, isDragging),
        testID: cellTestID,
    }
    const body = (
        <View
            className={boxClassFor(isSelected, Boolean(dropIndicator), Boolean(onPress))}
            style={BOX_BORDER_STYLE}
            testID={`${cellTestID}-box`}
        >
            <View className="flex-row items-center mb-1">
                {handle}
                <CellLabel fieldKey={fieldKey} label={label} isVisible={!hideLabel} />
                <RequiredMark isVisible={required} />
                {badge}
            </View>
            {children}
            <CellDescription text={description} />
            <CellError message={error} testID={`${cellTestID}-error`} />
            <DropBar fieldKey={fieldKey} side={dropIndicator} isFullWidth={effective === 'full'} />
        </View>
    )
    if (!onPress) return <View {...root}>{body}</View>
    return (
        // No button role: the cell holds the drag handle and the field's inputs, and RNW
        // renders a button element for that role, which cannot nest other buttons.
        <Pressable {...root} onPress={onPress} accessibilityLabel={`Select ${label}`}>
            {body}
        </Pressable>
    )
}
