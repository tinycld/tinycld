import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import { ArrowUpDown, Plus } from 'lucide-react-native'
import { type ReactElement, useRef, useState } from 'react'
import {
    type LayoutChangeEvent,
    type NativeScrollEvent,
    type NativeSyntheticEvent,
    Pressable,
    ScrollView,
    Text,
    View,
} from 'react-native'
import type { ReportViewModel } from '../hooks/useReportView'
import { type ControlItem, chipText, isDateParam, type ParamValue } from '../lib/params'
import type { ParamDef } from '../lib/spec'
import { sortChipLabel, sortChipText, sortChoices } from '../lib/view-url'
import { ParamChip } from './ParamChip'
import { ParamPeriodControl } from './ParamPeriodControl'
import { ParamPicker } from './ParamPickers'
import { REPORT_TAP_TARGET } from './ReportHeader'

const ROW_STYLE = { flexGrow: 0, flexShrink: 0 }
const CHIP_STYLE = { minHeight: REPORT_TAP_TARGET }

function openParamId(model: ReportViewModel): string | null {
    return model.surface?.kind === 'param' ? model.surface.paramId : null
}

interface SurfaceProps {
    model: ReportViewModel
    def: ParamDef
    value: ParamValue | null
    trigger: ReactElement
}

// The period control for a date param, the picker for any other.
function ParamSurface({ model, def, value, trigger }: SurfaceProps) {
    const isOpen = openParamId(model) === def.id
    const onOpenChange = (open: boolean) => {
        if (open) model.onOpenSurface({ kind: 'param', paramId: def.id })
        else model.onCloseSurfaceOf('param')
    }
    if (isDateParam(def)) {
        return (
            <ParamPeriodControl
                trigger={trigger}
                isOpen={isOpen}
                onOpenChange={onOpenChange}
                def={def}
                value={value}
                resolved={model.resolved[def.id]}
                onApply={(next, keepOpen) => model.onApplyParam(def.id, next, keepOpen)}
            />
        )
    }
    return (
        <ParamPicker
            trigger={trigger}
            isOpen={isOpen}
            onOpenChange={onOpenChange}
            def={def}
            value={value}
            onApply={next => model.onApplyParam(def.id, next)}
            source={model.optionsSource}
            params={model.postedParams}
        />
    )
}

interface ParamControlProps {
    model: ReportViewModel
    def: ParamDef
    value: ParamValue | null
    isRemovable: boolean
}

// The chip is handed to the surface as an element: the Popover clones it with the
// press that opens it and the ref it anchors on.
function ParamControl({ model, def, value, isRemovable }: ParamControlProps) {
    const chip = (
        <ParamChip
            paramId={def.id}
            text={chipText(def, value, model.optionLabels.get(def.id) ?? undefined)}
            label={def.label ?? def.id}
            isDate={isDateParam(def)}
            isRemovable={isRemovable}
            onRemove={() => model.onRemoveParam(def.id)}
        />
    )
    return <ParamSurface model={model} def={def} value={value} trigger={chip} />
}

// An element, not a component: the Menu and the Popover clone their trigger.
function addFilterChip(iconColor: string) {
    return (
        <Pressable
            testID="report-add-filter"
            accessibilityRole="button"
            accessibilityLabel="Add filter"
            className="flex-row items-center gap-1.5 rounded-full border border-border bg-secondary px-3"
            style={CHIP_STYLE}
        >
            <Plus size={14} color={iconColor} />
            <Text numberOfLines={1} className="text-sm text-secondary-foreground">
                Add filter
            </Text>
        </Pressable>
    )
}

// The picker of a param that is not set yet anchors on the "Add filter" chip, which
// then stands in for the menu that opened it.
function AddFilterControl({ model, defs }: { model: ReportViewModel; defs: ParamDef[] }) {
    const muted = useThemeColor('muted-foreground')
    const trigger = addFilterChip(muted)
    const picking = defs.find(def => def.id === openParamId(model))
    if (picking) return <ParamSurface model={model} def={picking} value={null} trigger={trigger} />
    const onOpenChange = (open: boolean) => {
        if (open) model.onOpenSurface({ kind: 'add-filter' })
        else model.onCloseSurfaceOf('add-filter')
    }
    const items = defs.map(def => (
        <Menu.Item
            key={def.id}
            label={def.label ?? def.id}
            testID={`report-add-filter-${def.id}`}
            onSelect={() => model.onOpenSurface({ kind: 'param', paramId: def.id })}
        />
    ))
    return (
        <Menu
            trigger={trigger}
            isOpen={model.surface?.kind === 'add-filter'}
            onOpenChange={onOpenChange}
            title="Add filter"
            sheetSide="bottom"
            testID="report-add-filter-menu"
        >
            {items}
        </Menu>
    )
}

function Control({ item, model }: { item: ControlItem; model: ReportViewModel }) {
    if (item.kind === 'add-filter') return <AddFilterControl model={model} defs={item.defs} />
    return (
        <ParamControl
            model={model}
            def={item.def}
            value={item.value}
            isRemovable={item.isRemovable}
        />
    )
}

// An element, not a component: the Menu clones its trigger.
function sortChip(text: string, label: string, iconColor: string) {
    return (
        <Pressable
            testID="report-sort-menu"
            accessibilityRole="button"
            accessibilityLabel={label}
            className="flex-row items-center gap-1.5 rounded-full border border-border bg-secondary px-3"
            style={CHIP_STYLE}
        >
            <ArrowUpDown size={14} color={iconColor} />
            <Text numberOfLines={1} className="text-sm text-secondary-foreground">
                {text}
            </Text>
        </Pressable>
    )
}

// D10: when the rows wrap, the headings are not buttons; the reader sorts here.
function SortControl({ model, isVisible }: { model: ReportViewModel; isVisible: boolean }) {
    const muted = useThemeColor('muted-foreground')
    const onSetSort = model.onSetSort
    if (!isVisible || !onSetSort) return null
    const { columns, display, sort } = model
    const trigger = sortChip(
        sortChipText(columns, display, sort),
        sortChipLabel(columns, display, sort),
        muted
    )
    const onOpenChange = (open: boolean) => {
        if (open) model.onOpenSurface({ kind: 'sort' })
        else model.onCloseSurfaceOf('sort')
    }
    const items = sortChoices(model.columns, model.display).map(choice => (
        <Menu.Item
            key={choice.key}
            label={choice.label}
            testID={`report-sort-choice-${choice.key}`}
            onSelect={() => onSetSort(choice.sort)}
        />
    ))
    return (
        <Menu
            trigger={trigger}
            isOpen={model.surface?.kind === 'sort'}
            onOpenChange={onOpenChange}
            title="Sort"
            sheetSide="bottom"
            testID="report-sort-menu-list"
        >
            {items}
        </Menu>
    )
}

// Opacities of the strips that fade the chip row's right edge, left to right.
const FADE_STEPS = [0.25, 0.5, 0.75, 0.95] as const
const FADE_STRIP = { width: 6 }
const FADE_STYLE = {
    position: 'absolute',
    top: 0,
    right: 0,
    bottom: 0,
    pointerEvents: 'none',
} as const

// D11: the chips keep their side scroll; while chips lie past the right edge, the
// edge fades so the reader sees there is more. Sizes are kept in a ref and only the
// answer is state, so a scroll redraws the row only when the fade comes or goes.
function useEdgeFade() {
    const sizes = useRef({ view: 0, content: 0, x: 0 })
    const [isShown, setIsShown] = useState(false)
    const update = (next: Partial<typeof sizes.current>) => {
        Object.assign(sizes.current, next)
        const { view, content, x } = sizes.current
        setIsShown(content - view - x > 1)
    }
    return {
        isShown,
        onLayout: (e: LayoutChangeEvent) => update({ view: e.nativeEvent.layout.width }),
        onContentSizeChange: (width: number) => update({ content: width }),
        onScroll: (e: NativeSyntheticEvent<NativeScrollEvent>) =>
            update({ x: e.nativeEvent.contentOffset.x }),
    }
}

const FADE_STRIPS = FADE_STEPS.map(opacity => (
    <View key={opacity} className="bg-background" style={[FADE_STRIP, { opacity }]} />
))

function EdgeFade({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View testID="report-controls-fade" className="flex-row" style={FADE_STYLE}>
            {FADE_STRIPS}
        </View>
    )
}

// One line of chips under the header: it scrolls sideways on a narrow screen and
// never wraps; a fade marks its right edge while more chips lie past it (D11).
export function ReportControls({
    model,
    isSortMenuShown,
}: {
    model: ReportViewModel
    isSortMenuShown: boolean
}) {
    const fade = useEdgeFade()
    if (model.controls.length === 0 && !isSortMenuShown) return null
    const controls = model.controls.map(item => (
        <Control
            key={item.kind === 'param' ? item.def.id : 'add-filter'}
            item={item}
            model={model}
        />
    ))
    return (
        <View style={ROW_STYLE}>
            <ScrollView
                horizontal
                testID="report-controls"
                showsHorizontalScrollIndicator={false}
                scrollEventThrottle={16}
                onLayout={fade.onLayout}
                onContentSizeChange={fade.onContentSizeChange}
                onScroll={fade.onScroll}
                contentContainerClassName="flex-row items-center gap-2 px-4 py-2"
            >
                {controls}
                <SortControl model={model} isVisible={isSortMenuShown} />
            </ScrollView>
            <EdgeFade isVisible={fade.isShown} />
        </View>
    )
}
