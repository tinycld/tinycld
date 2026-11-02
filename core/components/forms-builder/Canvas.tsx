import {
    addField,
    addPage,
    type DraftDefinition,
    moveField,
    newCustomField,
    normalizeDraft,
    removePage,
    renamePage,
} from '@tinycld/core/lib/forms/builder'
import type { CustomKind, FormDefinition, FormPage } from '@tinycld/core/lib/forms/definition'
import type { EvalContext, FormValues } from '@tinycld/core/lib/forms/logic'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { type DesignOptions, FormRenderer, useHostUi } from '@tinycld/core/ui/forms'
import { PromptDialog } from '@tinycld/core/ui/PromptDialog'
import { GripVertical } from 'lucide-react-native'
import { useMemo, useRef, useState } from 'react'
import { type Control, type FieldErrors, useForm, useWatch } from 'react-hook-form'
import { Platform, View } from 'react-native'
import { ZodError } from 'zod'
import { firstMessage } from './editor-errors'
import { KindPicker } from './KindPicker'
import { PageHeader } from './PageHeader'
import { type CanvasDrag, orderOf, useCanvasDrag, useDragHandle } from './use-canvas-drag'
import type { useEditorForm } from './use-editor-form'

type Editor = ReturnType<typeof useEditorForm>
type DraftField = DraftDefinition['fields'][number]

export interface CanvasProps {
    editor: Editor
    context: EvalContext
    /**
     * Created by the caller with `useEditorCanvasDrag`, so the canvas's
     * ScrollView can feed `drag.onScroll` and lock scrolling while
     * `drag.draggingKey` is set.
     */
    drag: CanvasDrag
}

/** The drag controller for an editor's canvas: moves land in the editor's draft. */
export function useEditorCanvasDrag(editor: Editor) {
    return useCanvasDrag({
        getOrder: () => orderOf(editor.form.getValues()),
        onMove: (key, page, index) => editor.apply(def => moveField(def, key, page, index)),
    })
}

/**
 * The draft as the renderer can draw it. A draft is briefly invalid while it
 * is edited (an options list emptied before a new row is typed); the canvas
 * then keeps the last valid definition instead of crashing. The watched draft
 * fields are returned too: editor errors are indexed by them.
 */
export function useCanvasDefinition(control: Control<DraftDefinition>) {
    const [version, pages, fields] = useWatch({ control, name: ['version', 'pages', 'fields'] })
    const lastGood = useRef<FormDefinition | null>(null)
    const definition = useMemo(() => {
        if (!pages || !fields) return lastGood.current
        try {
            lastGood.current = normalizeDraft({ version, pages, fields })
        } catch (error) {
            if (!(error instanceof ZodError)) throw error
        }
        return lastGood.current
    }, [version, pages, fields])
    return { definition, draftFields: fields ?? [] }
}

/**
 * Cell errors by field key. Errors are stored by the field's index in the
 * draft; the editor carries them along when a move or a removal shifts the
 * indexes, so the index each key has in the current draft finds its own.
 */
export function errorsByKey(errors: FieldErrors<DraftDefinition>, fields: DraftField[]) {
    const out: Record<string, string> = {}
    for (const [index, field] of fields.entries()) {
        const message = firstMessage(errors.fields?.[index])
        if (message) out[field.key] = message
    }
    return out
}

// Without touchAction a touch drag on web scrolls the page instead of moving the cell.
const HANDLE_STYLE = Platform.select({
    web: { touchAction: 'none', cursor: 'grab' } as Record<string, unknown>,
})

interface DragHandleProps {
    fieldKey: string
    label: string
    drag: CanvasDrag
}

function DragHandle({ fieldKey, label, drag }: DragHandleProps) {
    const { handlers } = useDragHandle(drag, fieldKey)
    const muted = useThemeColor('muted-foreground')
    return (
        <View
            {...handlers}
            accessibilityRole="button"
            accessibilityLabel={`Drag ${label}`}
            className="mr-1"
            style={HANDLE_STYLE}
            testID={`form-handle-${fieldKey}`}
        >
            <GripVertical size={14} color={muted} />
        </View>
    )
}

function deleteHintFor(hasFields: boolean) {
    return hasFields ? 'Move its fields first' : 'A form needs one page'
}

export interface PendingField {
    page: string
    kind: CustomKind
}

interface DesignInputs {
    editor: Editor
    definition: FormDefinition
    draftFields: DraftField[]
    drag: CanvasDrag
    /** A kind was picked: the canvas asks for the field's label before it adds the field. */
    onPickKind: (pending: PendingField) => void
}

function designOptionsFor(inputs: DesignInputs): DesignOptions {
    const { editor, definition, draftFields, drag, onPickKind } = inputs
    const hasFields = (page: FormPage) => definition.fields.some(f => f.page === page.key)
    const canDelete = (page: FormPage) => definition.pages.length > 1 && !hasFields(page)
    const labelOf = (key: string) => draftFields.find(f => f.key === key)?.label ?? key
    return {
        selectedKey: editor.selectedKey ?? undefined,
        onSelect: editor.select,
        renderHandle: key => <DragHandle fieldKey={key} label={labelOf(key)} drag={drag} />,
        renderPageHeader: page => (
            <PageHeader
                page={page}
                canDelete={canDelete(page)}
                deleteHint={deleteHintFor(hasFields(page))}
                onRename={title => editor.apply(def => renamePage(def, page.key, title))}
                onDelete={() => editor.applyRemoval(def => removePage(def, page.key))}
            />
        ),
        renderPageFooter: page => (
            <KindPicker
                onPick={kind => onPickKind({ page: page.key, kind })}
                kinds={editor.host.kinds}
                testID={`form-add-field-${page.key}`}
            />
        ),
        onCellLayout: drag.cellLayout,
        onPageLayout: drag.pageLayout,
        onCellMeasure: drag.registerCell,
        onPageMeasure: drag.registerPage,
        dropTarget: drag.draggingKey ? drag.dropTarget : null,
        draggingKey: drag.draggingKey ?? undefined,
        errors: errorsByKey(editor.form.formState.errors, draftFields),
    }
}

/** Where a new field lands on its page: before the host's named built-in, else the page end. */
function landingIndex(def: DraftDefinition, page: string, before?: string) {
    if (!before) return undefined
    const index = def.fields.findIndex(f => f.page === page && f.key === before)
    return index === -1 ? undefined : index
}

/** The field a picked kind becomes once its label is typed; cancelling adds nothing. */
function useAddField(editor: Editor) {
    // The kind waiting for its label is local UI state.
    const [pending, setPending] = useState<PendingField | null>(null)
    const add = (label: string) => {
        if (!pending) return
        const taken = new Set(editor.form.getValues('fields').map(f => f.key))
        const field = newCustomField(pending.kind, label, pending.page, taken)
        editor.apply(def =>
            addField(def, field, landingIndex(def, pending.page, editor.host?.insertCustomBefore))
        )
        editor.select(field.key)
        setPending(null)
    }
    return { isAsking: pending !== null, ask: setPending, cancel: () => setPending(null), add }
}

export function Canvas({ editor, context, drag }: CanvasProps) {
    const { definition, draftFields } = useCanvasDefinition(editor.form.control)
    // FormRenderer's inputs need a form instance; in design mode its values never matter.
    const inert = useForm<FormValues>({ defaultValues: {} })
    const adding = useAddField(editor)
    const ui = useHostUi(editor.host ?? undefined)
    const addNewPage = () => editor.apply(def => addPage(def, 'New page'))
    if (!editor.host || !definition) return null
    const design = designOptionsFor({
        editor,
        definition,
        draftFields,
        drag,
        onPickKind: adding.ask,
    })
    return (
        <View testID="form-canvas">
            <FormRenderer
                host={editor.host}
                ui={ui}
                definition={definition}
                layout="design"
                form={inert}
                context={context}
                design={design}
            />
            <Button
                variant="ghost"
                size="sm"
                onPress={addNewPage}
                className="self-start mt-2"
                testID="form-add-page"
            >
                <ButtonText>Add page</ButtonText>
            </Button>
            <PromptDialog
                isOpen={adding.isAsking}
                onClose={adding.cancel}
                onSubmit={adding.add}
                title="Field label"
                placeholder="Severity"
                confirmLabel="Add"
                required
                maxLength={120}
                requiresServer={false}
            />
        </View>
    )
}
