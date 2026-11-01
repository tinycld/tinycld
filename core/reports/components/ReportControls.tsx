import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import { Plus } from 'lucide-react-native'
import type { ReactElement } from 'react'
import { Pressable, ScrollView, Text } from 'react-native'
import type { ReportViewModel } from '../hooks/useReportView'
import { type ControlItem, chipText, isDateParam, type ParamValue } from '../lib/params'
import type { ParamDef } from '../lib/spec'
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
            text={chipText(def, value, model.resolved[def.id])}
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

// One line of chips under the header: it scrolls sideways on a narrow screen and
// never wraps.
export function ReportControls({ model }: { model: ReportViewModel }) {
    if (model.controls.length === 0) return null
    const controls = model.controls.map(item => (
        <Control
            key={item.kind === 'param' ? item.def.id : 'add-filter'}
            item={item}
            model={model}
        />
    ))
    return (
        <ScrollView
            horizontal
            testID="report-controls"
            showsHorizontalScrollIndicator={false}
            style={ROW_STYLE}
            contentContainerClassName="flex-row items-center gap-2 px-4 py-2"
        >
            {controls}
        </ScrollView>
    )
}
