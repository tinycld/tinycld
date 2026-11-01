import { useLeaveGuard } from '@tinycld/core/components/forms-builder/use-leave-guard'
import {
    type Breakpoint,
    useBreakpoint,
    useWindowHeight,
} from '@tinycld/core/components/workspace/useBreakpoint'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useDeviceInsets } from '@tinycld/core/lib/use-safe-area'
import { Button, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { ConfirmDialog } from '@tinycld/core/ui/ConfirmDialog'
import {
    Drawer,
    DrawerBackdrop,
    DrawerBody,
    DrawerContent,
    DrawerFooter,
    DrawerHeader,
} from '@tinycld/core/ui/drawer'
import { PromptDialog } from '@tinycld/core/ui/PromptDialog'
import { Sheet } from '@tinycld/core/ui/sheet'
import { X } from 'lucide-react-native'
import { type ReactNode, useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { copyTitle, type ReportBuilder } from '../../hooks/useReportBuilder'
import type { ReportViewModel } from '../../hooks/useReportView'
import type { Problem } from '../../lib/types'
import { REPORT_TAP_TARGET } from '../ReportHeader'
import { ReportBody } from '../ReportView'
import { ColumnsSection } from './ColumnsSection'
import { DetailsSection } from './DetailsSection'

const DISCARD = {
    title: 'Discard changes?',
    message: 'Your changes to this report are not saved.',
}

function CloseButton({ onPress }: { onPress: () => void }) {
    const color = useThemeColor('foreground')
    return (
        <Pressable
            testID="report-builder-close"
            accessibilityRole="button"
            accessibilityLabel="Close"
            onPress={onPress}
            className="items-center justify-center"
            style={{ width: REPORT_TAP_TARGET, height: REPORT_TAP_TARGET }}
        >
            <X size={18} color={color} />
        </Pressable>
    )
}

const PANEL_TITLES = { new: 'New report', edit: 'Edit report' } as const

function PanelTitle({ builder }: { builder: ReportBuilder }) {
    return (
        <Text className="text-lg font-semibold text-foreground">{PANEL_TITLES[builder.mode]}</Text>
    )
}

// The sections, in the order a report is designed; the later sections join them here.
function Sections({ builder, problems }: { builder: ReportBuilder; problems: Problem[] }) {
    return (
        <View>
            <DetailsSection builder={builder} problems={problems} />
            <ColumnsSection builder={builder} problems={problems} />
        </View>
    )
}

function SaveAsButton({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    if (!isVisible) return null
    return (
        <Button testID="report-builder-save-as" variant="outline" onPress={onPress}>
            <ButtonText>Save as…</ButtonText>
        </Button>
    )
}

function Footer({ builder, onSaveAs }: { builder: ReportBuilder; onSaveAs: () => void }) {
    return (
        <View className="flex-row justify-end gap-2 border-t border-border p-3">
            <SaveAsButton isVisible={builder.mode === 'edit'} onPress={onSaveAs} />
            <ServerActionButton
                testID="report-builder-save"
                onPress={builder.onSave}
                isDisabled={builder.isSaving}
            >
                <ButtonText>Save</ButtonText>
            </ServerActionButton>
        </View>
    )
}

interface SurfaceProps {
    builder: ReportBuilder
    preview: ReportViewModel
    problems: Problem[]
    onSaveAs: () => void
}

// Beside the report on a desktop; over it, from the right, on a tablet. The panel
// belongs to its route, so it does not close on a URL change: its close navigates,
// and a close fired by the next push would undo that push.
function DrawerSurface({
    builder,
    problems,
    onSaveAs,
    breakpoint,
}: SurfaceProps & { breakpoint: Breakpoint }) {
    const isInline = breakpoint === 'desktop'
    const presentation = isInline ? 'inline' : 'overlay'
    const role = isInline ? 'complementary' : 'dialog'
    return (
        <Drawer
            isOpen
            onClose={builder.onClose}
            presentation={presentation}
            closesOnNavigate={false}
            anchor="right"
            size="md"
        >
            <DrawerBackdrop />
            <DrawerContent
                testID="report-builder"
                role={role}
                accessibilityLabel="Report builder"
                className="w-[440px]"
            >
                <DrawerHeader className="border-b border-border">
                    <PanelTitle builder={builder} />
                    <CloseButton onPress={builder.onClose} />
                </DrawerHeader>
                <DrawerBody style={{ flexShrink: 1 }}>
                    <Sections builder={builder} problems={problems} />
                </DrawerBody>
                <DrawerFooter>
                    <Footer builder={builder} onSaveAs={onSaveAs} />
                </DrawerFooter>
            </DrawerContent>
        </Drawer>
    )
}

const MODES = [
    { value: 'design', label: 'Design' },
    { value: 'preview', label: 'Preview' },
] as const
type Mode = (typeof MODES)[number]['value']

function ModePill({
    item,
    isSelected,
    onChange,
}: {
    item: (typeof MODES)[number]
    isSelected: boolean
    onChange: (mode: Mode) => void
}) {
    const pillClass = isSelected ? 'border-primary bg-primary' : 'border-border'
    const textClass = isSelected ? 'text-primary-foreground' : 'text-foreground'
    return (
        <Pressable
            testID={`report-builder-mode-${item.value}`}
            accessibilityRole="radio"
            aria-checked={isSelected}
            onPress={() => onChange(item.value)}
            className={`flex-1 items-center justify-center rounded-full border px-3 ${pillClass}`}
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <Text className={`text-sm font-medium ${textClass}`}>{item.label}</Text>
        </Pressable>
    )
}

function ModeSwitch({ mode, onChange }: { mode: Mode; onChange: (mode: Mode) => void }) {
    const pills = MODES.map(item => (
        <ModePill
            key={item.value}
            item={item}
            isSelected={item.value === mode}
            onChange={onChange}
        />
    ))
    return (
        <View accessibilityRole="radiogroup" className="flex-row gap-2 px-3 pb-2">
            {pills}
        </View>
    )
}

function ModeBody({ isVisible, children }: { isVisible: boolean; children: ReactNode }) {
    if (!isVisible) return null
    return <View className="min-h-0 flex-1">{children}</View>
}

// A phone cannot show the design and the report together, so a full-height sheet
// switches between them. The switch is this sheet's own UI state. A sheet takes its
// content's height, so the content is given the screen's.
function SheetSurface({ builder, preview, problems, onSaveAs }: SurfaceProps) {
    const [mode, setMode] = useState<Mode>('design')
    const insets = useDeviceInsets()
    const height = useWindowHeight() - insets.top
    return (
        <Sheet
            isOpen
            onClose={builder.onClose}
            maxHeightPercent={1}
            hideHandle
            testID="report-builder"
        >
            <View style={{ height }}>
                <View className="flex-row items-center justify-between px-3">
                    <PanelTitle builder={builder} />
                    <CloseButton onPress={builder.onClose} />
                </View>
                <ModeSwitch mode={mode} onChange={setMode} />
                <ModeBody isVisible={mode === 'design'}>
                    <ScrollView contentContainerClassName="px-3">
                        <Sections builder={builder} problems={problems} />
                    </ScrollView>
                </ModeBody>
                <ModeBody isVisible={mode === 'preview'}>
                    <ReportBody model={preview} />
                </ModeBody>
                <Footer builder={builder} onSaveAs={onSaveAs} />
            </View>
        </Sheet>
    )
}

function BuilderSurface(props: SurfaceProps) {
    const breakpoint = useBreakpoint()
    if (breakpoint === 'mobile') return <SheetSurface {...props} />
    return <DrawerSurface {...props} breakpoint={breakpoint} />
}

export function ReportBuilderPanel({
    builder,
    preview,
}: {
    builder: ReportBuilder
    preview: ReportViewModel
}) {
    // A dirty builder asks before its route is left; a save in flight or done does not.
    const leave = useLeaveGuard(builder.isDirty && !builder.isSaving && !builder.isSaved)
    // The Save as prompt is a modal toggle: local UI state.
    const [isSavingAs, setSavingAs] = useState(false)
    const problems = [...builder.saveProblems, ...preview.problems]
    const onSubmitCopy = (title: string) => {
        setSavingAs(false)
        builder.onSaveAs(title)
    }
    return (
        <>
            <BuilderSurface
                builder={builder}
                preview={preview}
                problems={problems}
                onSaveAs={() => setSavingAs(true)}
            />
            <ConfirmDialog
                isOpen={builder.isConfirmingDiscard}
                onClose={builder.onKeepEditing}
                onConfirm={builder.onDiscard}
                title={DISCARD.title}
                message={DISCARD.message}
                confirmLabel="Discard"
                cancelLabel="Keep editing"
                isDestructive
                requiresServer={false}
            />
            <ConfirmDialog
                isOpen={leave.isConfirming}
                onClose={leave.cancel}
                onConfirm={leave.discard}
                title={DISCARD.title}
                message={DISCARD.message}
                confirmLabel="Discard"
                cancelLabel="Keep editing"
                isDestructive
                requiresServer={false}
            />
            <PromptDialog
                isOpen={isSavingAs}
                onClose={() => setSavingAs(false)}
                onSubmit={onSubmitCopy}
                title="Save as"
                placeholder="Report title"
                confirmLabel="Save copy"
                defaultValue={copyTitle(builder.title)}
                maxLength={200}
                required
            />
        </>
    )
}
