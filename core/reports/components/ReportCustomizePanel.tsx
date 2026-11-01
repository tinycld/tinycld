import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import {
    Drawer,
    DrawerBackdrop,
    DrawerBody,
    DrawerContent,
    DrawerHeader,
} from '@tinycld/core/ui/drawer'
import { ButtonGroup, Toggle, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { Sheet } from '@tinycld/core/ui/sheet'
import { ChevronDown, ChevronRight, X } from 'lucide-react-native'
import { type ReactNode, useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import type { ReportViewModel } from '../hooks/useReportView'
import { mergeHidden } from '../lib/format'
import { REPORT_TAP_TARGET } from './ReportHeader'

const NEGATIVE_OPTIONS = [
    { value: 'minus', label: '-100' },
    { value: 'parens', label: '(100)' },
]
const CLOSE_TARGET = { width: REPORT_TAP_TARGET, height: REPORT_TAP_TARGET }
const TARGET_STYLE = { minHeight: REPORT_TAP_TARGET }

const displaySchema = z.object({
    divideBy1000: z.boolean(),
    withoutCents: z.boolean(),
    negative: z.enum(['minus', 'parens']),
    negativeRed: z.boolean(),
    columns: z.array(z.object({ id: z.string(), label: z.string(), isShown: z.boolean() })),
})
type DisplayValues = z.infer<typeof displaySchema>

// Customize is a form over the URL's display options: `values` keeps it in step with
// the URL, and every change writes the URL at once (live apply, no Save). The first
// column names the rows, so it cannot be hidden.
function useDisplayForm(model: ReportViewModel) {
    const values: DisplayValues = {
        divideBy1000: model.display.divideBy1000,
        withoutCents: model.display.withoutCents,
        negative: model.display.negative,
        negativeRed: model.display.negativeRed,
        columns: model.allColumns.slice(1).map(column => ({
            id: column.id,
            label: column.label,
            isShown: !model.display.hidden.includes(column.id),
        })),
    }
    const form = useForm<DisplayValues>({ resolver: zodResolver(displaySchema), values })
    const write = () =>
        void form.handleSubmit(next =>
            model.onDisplayChange({
                divideBy1000: next.divideBy1000,
                withoutCents: next.withoutCents,
                negative: next.negative,
                negativeRed: next.negativeRed,
                hidden: mergeHidden(model.display.hidden, next.columns),
            })
        )()
    return { control: form.control, columns: values.columns, write }
}
type DisplayForm = ReturnType<typeof useDisplayForm>

function SectionBody({ isVisible, children }: { isVisible: boolean; children: ReactNode }) {
    if (!isVisible) return null
    return <View className="pb-2">{children}</View>
}

interface PanelSectionProps {
    id: string
    title: string
    isOpenAtStart: boolean
    children: ReactNode
}

function PanelSection({ id, title, isOpenAtStart, children }: PanelSectionProps) {
    // The fold state is this panel's own: nothing else reads it.
    const [isOpen, setIsOpen] = useState(isOpenAtStart)
    const color = useThemeColor('muted-foreground')
    const Caret = isOpen ? ChevronDown : ChevronRight
    return (
        <View className="border-b border-border">
            <Pressable
                testID={`report-customize-section-${id}`}
                accessibilityRole="button"
                aria-expanded={isOpen}
                onPress={() => setIsOpen(open => !open)}
                className="flex-row items-center gap-2"
                style={TARGET_STYLE}
            >
                <Caret size={16} color={color} />
                <Text className="text-sm font-semibold text-foreground">{title}</Text>
            </Pressable>
            <SectionBody isVisible={isOpen}>{children}</SectionBody>
        </View>
    )
}

function NumberFormatFields({ form }: { form: DisplayForm }) {
    return (
        <>
            <Toggle
                name="divideBy1000"
                control={form.control}
                label="Divide by 1000"
                testID="report-option-divide-by-1000"
                onValueChange={form.write}
            />
            <Toggle
                name="withoutCents"
                control={form.control}
                label="Without cents"
                testID="report-option-without-cents"
                onValueChange={form.write}
            />
            <ButtonGroup
                name="negative"
                control={form.control}
                label="Negative numbers"
                horizontal
                options={NEGATIVE_OPTIONS}
                onValueChange={form.write}
            />
            <Toggle
                name="negativeRed"
                control={form.control}
                label="Show negative numbers in red"
                testID="report-option-negative-red"
                onValueChange={form.write}
            />
        </>
    )
}

function ColumnFields({ form }: { form: DisplayForm }) {
    const toggles = form.columns.map((column, index) => (
        <Toggle
            key={column.id}
            name={`columns.${index}.isShown`}
            control={form.control}
            label={column.label}
            testID={`report-option-column-${column.id}`}
            onValueChange={form.write}
        />
    ))
    return <>{toggles}</>
}

function CustomizeFields({ model }: { model: ReportViewModel }) {
    const form = useDisplayForm(model)
    return (
        <View>
            <PanelSection id="number-format" title="Number format" isOpenAtStart>
                <NumberFormatFields form={form} />
            </PanelSection>
            <PanelSection id="columns" title="Columns" isOpenAtStart={false}>
                <ColumnFields form={form} />
            </PanelSection>
            <View className="items-start pt-3">
                <Button
                    testID="report-customize-reset"
                    variant="outline"
                    style={TARGET_STYLE}
                    onPress={model.onResetDisplay}
                >
                    <ButtonText>Reset to defaults</ButtonText>
                </Button>
            </View>
        </View>
    )
}

function PanelTitle() {
    return <Text className="text-lg font-semibold text-foreground">Customize</Text>
}

function CloseButton({ onPress }: { onPress: () => void }) {
    const color = useThemeColor('foreground')
    return (
        <Pressable
            testID="report-customize-close"
            accessibilityRole="button"
            accessibilityLabel="Close"
            onPress={onPress}
            className="items-center justify-center"
            style={CLOSE_TARGET}
        >
            <X size={18} color={color} />
        </Pressable>
    )
}

function CustomizeSheet({ model }: { model: ReportViewModel }) {
    return (
        <Sheet
            isOpen={model.isCustomizeOpen}
            onClose={model.onCloseCustomize}
            testID="report-customize"
        >
            <Sheet.Body contentClassName="px-4 pb-4">
                <View className="flex-row items-center justify-between">
                    <PanelTitle />
                    <CloseButton onPress={model.onCloseCustomize} />
                </View>
                <CustomizeFields model={model} />
            </Sheet.Body>
        </Sheet>
    )
}

// Wide: inline beside the report, which updates behind it. Tablet: an overlay.
function CustomizeDrawer({ model }: { model: ReportViewModel }) {
    const isInline = model.breakpoint === 'desktop'
    const presentation = isInline ? 'inline' : 'overlay'
    const role = isInline ? 'complementary' : 'dialog'
    return (
        <Drawer
            isOpen={model.isCustomizeOpen}
            onClose={model.onCloseCustomize}
            presentation={presentation}
            anchor="right"
            size="md"
        >
            <DrawerBackdrop />
            <DrawerContent
                testID="report-customize"
                role={role}
                accessibilityLabel="Customize"
                className="w-[360px]"
            >
                <DrawerHeader className="border-b border-border">
                    <PanelTitle />
                    <CloseButton onPress={model.onCloseCustomize} />
                </DrawerHeader>
                <DrawerBody style={{ flexShrink: 1 }}>
                    <CustomizeFields model={model} />
                </DrawerBody>
            </DrawerContent>
        </Drawer>
    )
}

/** Customize: a sheet on a phone, a panel beside or over the report on wider screens. */
export function ReportCustomizePanel({ model }: { model: ReportViewModel }) {
    if (model.isEditing) return null
    if (model.breakpoint === 'mobile') return <CustomizeSheet model={model} />
    return <CustomizeDrawer model={model} />
}
