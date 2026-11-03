import { HelpIcon } from '@tinycld/core/components/help/HelpIcon'
import { SetupTabs } from '@tinycld/core/components/setup/wizard/SetupTabs'
import { useAuth } from '@tinycld/core/lib/auth'
import { toDateString } from '@tinycld/core/lib/dates'
import { WEB_FORM_HOST_KEY } from '@tinycld/core/lib/forms/core-hosts'
import type { EvalContext } from '@tinycld/core/lib/forms/logic'
import { type FormHost, useFormHost } from '@tinycld/core/lib/forms/use-form-hosts'
import { type FormRow, useFormRow } from '@tinycld/core/lib/forms/use-forms-of-host'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { Button, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { ConfirmDialog } from '@tinycld/core/ui/ConfirmDialog'
import { ArrowLeft } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { Canvas, useEditorCanvasDrag } from './Canvas'
import { DragGhost } from './DragGhost'
import { EditorPanel } from './EditorPanel'
import { describePath, errorLeaves } from './editor-errors'
import { FieldProperties } from './FieldProperties'
import { PreviewDialog } from './PreviewDialog'
import { PublishTab } from './PublishTab'
import { RulesTab } from './RulesTab'
import { SubmissionsTab } from './SubmissionsTab'
import type { CanvasDrag } from './use-canvas-drag'
import { useEditorForm } from './use-editor-form'
import { useLeaveGuard } from './use-leave-guard'

type Editor = ReturnType<typeof useEditorForm>
type Tab = 'fields' | 'rules' | 'publish' | 'submissions'

const BASE_TABS = [
    { value: 'fields', label: 'Fields' },
    { value: 'rules', label: 'Rules' },
] as const
const WEB_FORM_TABS = [
    ...BASE_TABS,
    { value: 'publish', label: 'Publish' },
    { value: 'submissions', label: 'Submissions' },
] as const

// Only a web form is published; other hosts' forms live inside their package.
function tabsFor(host: FormHost) {
    return host.key === WEB_FORM_HOST_KEY ? WEB_FORM_TABS : BASE_TABS
}

function useFormEditor(row: FormRow, host: FormHost) {
    const editor = useEditorForm(row, host)
    const drag = useEditorCanvasDrag(editor)
    const { user } = useAuth()
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings/forms'))
    const leave = useLeaveGuard(editor.isDirty)
    // Active tab and whether the preview is open are local UI state.
    const [tab, setTab] = useState<Tab>('fields')
    const [isPreviewing, setIsPreviewing] = useState(false)
    const context: EvalContext = { today: toDateString(new Date()), userId: user.id }
    return {
        editor,
        drag,
        context,
        tab,
        setTab,
        isPreviewing,
        openPreview: () => setIsPreviewing(true),
        closePreview: () => setIsPreviewing(false),
        isConfirmingLeave: leave.isConfirming,
        cancelLeave: leave.cancel,
        discard: leave.discard,
        // A dirty editor is held back by the leave guard, which asks first.
        back: navigateBack,
        save: () => {
            void editor.saveSafely()
        },
    }
}

function Notice({ text, onDismiss }: { text: string | null; onDismiss: () => void }) {
    if (!text) return null
    return (
        <Pressable
            onPress={onDismiss}
            accessibilityLabel={`Dismiss: ${text}`}
            className="bg-muted rounded-md px-3 py-2 mb-2"
            testID="form-notice"
        >
            <Text className="text-foreground text-sm">{text}</Text>
        </Pressable>
    )
}

function ChangedElsewhere({ isVisible, onReload }: { isVisible: boolean; onReload: () => void }) {
    if (!isVisible) return null
    return (
        <View
            className="flex-row items-center gap-3 bg-muted rounded-md px-3 py-2 mb-2"
            testID="form-changed-elsewhere"
        >
            <Text className="text-foreground text-sm flex-1">
                This form was changed elsewhere. Reload to see it.
            </Text>
            <Button variant="outline" size="sm" onPress={onReload} testID="form-reload">
                <ButtonText>Reload</ButtonText>
            </Button>
        </View>
    )
}

function errorLines(editor: Editor) {
    const draft = editor.form.getValues()
    return errorLeaves(editor.form.formState.errors).map(leaf => {
        const where = describePath(leaf.path, draft)
        return {
            id: `${leaf.path}:${leaf.message}`,
            text: where ? `${where}: ${leaf.message}` : leaf.message,
        }
    })
}

/** The editor's errors, each named the way the admin sees it rather than by its path. */
function EditorErrors({ editor }: { editor: Editor }) {
    const lines = errorLines(editor)
    if (!lines.length) return null
    const items = lines.map(line => (
        <Text key={line.id} className="text-xs text-danger mb-1">
            {line.text}
        </Text>
    ))
    return (
        <View
            className="bg-danger-soft border border-danger-soft-foreground rounded-lg p-3 mb-3"
            testID="form-errors"
        >
            <Text className="text-sm font-semibold text-danger mb-2">
                The form cannot be saved yet
            </Text>
            {items}
        </View>
    )
}

interface HeaderProps {
    title: string
    isDirty: boolean
    isSaving: boolean
    onBack: () => void
    onPreview: () => void
    onSave: () => void
}

function EditorHeader({ title, isDirty, isSaving, onBack, onPreview, onSave }: HeaderProps) {
    const fg = useThemeColor('foreground')
    return (
        <View className="flex-row items-center gap-3 p-5 pb-2">
            <Pressable onPress={onBack} accessibilityLabel="Back" testID="form-back">
                <ArrowLeft size={24} color={fg} />
            </Pressable>
            <Text className="text-foreground text-[22px] font-bold flex-1" numberOfLines={1}>
                {title}
            </Text>
            <HelpIcon topic="core:forms" />
            <Button variant="outline" size="sm" onPress={onPreview} testID="form-preview">
                <ButtonText>Preview</ButtonText>
            </Button>
            <ServerActionButton
                size="sm"
                onPress={onSave}
                isDisabled={!isDirty || isSaving}
                testID="form-save"
            >
                <ButtonText>{isSaving ? 'Saving…' : 'Save'}</ButtonText>
            </ServerActionButton>
        </View>
    )
}

interface TabBodyProps {
    editor: Editor
    context: EvalContext
    drag: CanvasDrag
    isVisible: boolean
}

function FieldsTab({ editor, context, drag, isVisible }: TabBodyProps) {
    if (!isVisible) return null
    return <Canvas editor={editor} context={context} drag={drag} />
}

/** What the drag ghost says: the dragged field's current label. */
function draggedLabel(editor: Editor, key: string | null) {
    if (!key) return ''
    return editor.form.getValues('fields').find(f => f.key === key)?.label ?? key
}

function SelectedProperties({ editor }: { editor: Editor }) {
    if (!editor.selectedKey) return null
    return <FieldProperties editor={editor} fieldKey={editor.selectedKey} />
}

function NotFound() {
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings/forms'))
    const fg = useThemeColor('foreground')
    return (
        <View className="flex-1 bg-background p-5 gap-3" testID="form-not-found">
            <Pressable onPress={navigateBack} accessibilityLabel="Back" testID="form-back">
                <ArrowLeft size={24} color={fg} />
            </Pressable>
            <Text className="text-muted-foreground">
                This form does not exist or its package is not installed.
            </Text>
        </View>
    )
}

/** Mounts the editor only once the row and its host are known, so the draft is read from a loaded row. */
export function FormEditor({ formId }: { formId: string }) {
    const { row, isReady } = useFormRow(formId)
    const host = useFormHost(row?.host)
    if (!isReady) return null
    if (!row || !host) return <NotFound />
    return <LoadedEditor key={formId} row={row} host={host} />
}

function LoadedEditor({ row, host }: { row: FormRow; host: FormHost }) {
    const s = useFormEditor(row, host)
    const { editor, drag } = s
    const isPanelOpen = s.tab === 'fields' && editor.selectedKey !== null
    return (
        <View className="flex-1 bg-background" testID="form-editor">
            <EditorHeader
                title={row.name}
                isDirty={editor.isDirty}
                isSaving={editor.isSaving}
                onBack={s.back}
                onPreview={s.openPreview}
                onSave={s.save}
            />
            <View className="px-5">
                <SetupTabs
                    tabs={tabsFor(host)}
                    value={s.tab}
                    onChange={s.setTab}
                    testID="form-tab"
                />
                <ChangedElsewhere isVisible={editor.isChangedElsewhere} onReload={editor.reload} />
                <EditorErrors editor={editor} />
                <Notice text={editor.notice} onDismiss={editor.dismissNotice} />
            </View>
            <View className="flex-1 flex-row px-5">
                <ScrollView
                    className="flex-1"
                    contentContainerClassName="pr-5"
                    onScroll={drag.onScroll}
                    scrollEventThrottle={16}
                    scrollEnabled={!drag.draggingKey}
                >
                    <FieldsTab
                        editor={editor}
                        context={s.context}
                        drag={drag}
                        isVisible={s.tab === 'fields'}
                    />
                    <RulesTab editor={editor} isVisible={s.tab === 'rules'} />
                    <PublishTab
                        formId={row.id}
                        formName={row.name}
                        isVisible={s.tab === 'publish'}
                    />
                    <SubmissionsTab
                        formId={row.id}
                        definition={row.definition}
                        isVisible={s.tab === 'submissions'}
                    />
                </ScrollView>
                <EditorPanel isOpen={isPanelOpen} onClose={() => editor.select(null)} title="Field">
                    <SelectedProperties editor={editor} />
                </EditorPanel>
            </View>
            <PreviewDialog
                editor={editor}
                context={s.context}
                isOpen={s.isPreviewing}
                onClose={s.closePreview}
            />
            <ConfirmDialog
                isOpen={s.isConfirmingLeave}
                onClose={s.cancelLeave}
                onConfirm={s.discard}
                title="Discard changes?"
                message="This form has changes that are not saved."
                confirmLabel="Discard"
                isDestructive
                requiresServer={false}
            />
            <DragGhost drag={drag} label={draggedLabel(editor, drag.draggingKey)} />
        </View>
    )
}
