import type { FormDefinition, FormField, FormPage } from '@tinycld/core/lib/forms/definition'
import type { CollabDoc, FieldMode, FormHostUi } from '@tinycld/core/lib/forms/host-ui'
import type { EvalContext, FormState, FormValues } from '@tinycld/core/lib/forms/logic'
import type { HostSchemas } from '@tinycld/core/lib/forms/values'
import type { FormHost } from '@tinycld/core/lib/packages/derive-form-hosts'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { FormErrorSummary } from '@tinycld/core/ui/form'
import { type MutableRefObject, type ReactElement, type ReactNode, useRef, useState } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { type LayoutChangeEvent, ScrollView, Text, View } from 'react-native'
import { FieldCell } from './FieldCell'
import { FieldInput, hasPinnedRow, PinnedHostRow, rendersBare, rendersOwnLabel } from './FieldInput'
import { FormLayout } from './FormLayout'
import { type SubmitSummaryKind, submitSummary } from './submit-summary'
import { useContainerWidth } from './use-container-width'
import { useFormState } from './use-form-state'
import { type LayoutRect, type MeasureRect, useMeasuredRect } from './use-measure-rect'
import type { FormSchema } from './use-schema-ref-resolver'
import { WizardSteps } from './WizardSteps'

/** A stable empty object, so the schema memo does not rebuild when the host has no schemas. */
const NO_SCHEMAS: HostSchemas = {}

export type FormLayoutKind = 'wizard' | 'sections' | 'design'

export interface DesignOptions {
    selectedKey?: string
    onSelect?: (key: string) => void
    renderHandle?: (key: string) => ReactNode
    /** Replaces the section title. */
    renderPageHeader?: (page: FormPage) => ReactNode
    /** After a page's fields, e.g. an "Add field" button. */
    renderPageFooter?: (page: FormPage) => ReactNode
    onCellLayout?: (key: string, rect: LayoutRect) => void
    onPageLayout?: (pageKey: string, rect: LayoutRect) => void
    /**
     * Once per mounted cell: a thunk that re-measures it now. Returns the
     * unregister function, called on unmount. `onCellLayout` misses a cell
     * that moved without resizing, so the canvas re-measures at drag start.
     */
    onCellMeasure?: (key: string, measure: MeasureRect) => () => void
    onPageMeasure?: (pageKey: string, measure: MeasureRect) => () => void
    dropTarget?: DropTarget | null
    /** The field being dragged; its cell dims. */
    draggingKey?: string
    badges?: Record<string, ReactNode>
    /** Cell error text keyed by field key, from the editor form. */
    errors?: Record<string, string>
}

export type DropTarget = { key: string; side: 'before' | 'after' } | { page: string; end: true }

/** The dragged cell marks its own edge too: the bar then says the field stays where it is, instead of vanishing. */
function dropSideFor(design: DesignOptions | undefined, key: string) {
    const target = design?.draggingKey ? design.dropTarget : null
    if (!target || !('key' in target) || target.key !== key) return undefined
    return target.side
}

export interface SectionsScroll {
    /** Class for every row's wrapper: the content column's padding and width. */
    rowClassName?: string
}

export interface FormRendererProps {
    host: FormHost
    ui: FormHostUi | null
    definition: FormDefinition
    layout: FormLayoutKind
    form: UseFormReturn<FormValues>
    context: EvalContext
    recordId?: string
    readOnly?: boolean
    collab?: CollabDoc | null
    /** Wizard: called after the last page validates. */
    /** Wizard: called after the last page validates; Send stays off until a returned promise settles. */
    onSubmit?: () => unknown
    submitLabel?: string
    design?: DesignOptions
    /** Sections only: the renderer owns a ScrollView whose direct children are the rows, so pinned rows can stick. */
    scroll?: SectionsScroll
    /** Kept current with the effective schema; see useSchemaRefResolver. */
    schemaRef?: MutableRefObject<FormSchema>
    /**
     * `all` (default): the error summary lists every error after a submit.
     * `root`: it lists only the no-field (server) error, whenever one exists,
     * for a host that shows field errors inline only and saves edits without a
     * submit. See submitSummary.
     */
    summary?: SubmitSummaryKind
    /** Create mode with no session (a public form): file fields hold picked files until submit. */
    pendingFiles?: boolean
    testID?: string
}

interface BodyProps extends FormRendererProps {
    state: FormState
    computed: FormValues
    containerWidth: number
    mode: FieldMode
    /** Enter in a single-line input; set by the wizard. */
    onSubmitEditing?: () => void
    /** Measures the content column; the sections layout attaches it inside the row padding. */
    onContainerLayout: (e: LayoutChangeEvent) => void
}

interface PageProps extends BodyProps {
    page: FormPage
}

interface PageChromeProps {
    page: FormPage
    design?: DesignOptions
}

interface CellProps extends BodyProps {
    field: FormField
}

function visibleFieldsOf(
    def: FormDefinition,
    page: FormPage,
    state: FormState,
    mode: FieldMode
): FormField[] {
    return def.fields.filter(
        f => f.page === page.key && (mode === 'design' || state.fields[f.key]?.visible)
    )
}

function Cell(props: CellProps) {
    const { field, state, containerWidth, host, ui, form, mode, recordId, collab, design } = props
    const hideLabel = rendersOwnLabel(host, field, ui, mode)
    const onCellMeasure = design?.onCellMeasure
    const fieldState = state.fields[field.key]
    const formError = form.formState.errors[field.key]?.message
    const error =
        design?.errors?.[field.key] ?? (typeof formError === 'string' ? formError : undefined)
    return (
        <FieldCell
            fieldKey={field.key}
            label={field.label}
            description={field.description}
            required={fieldState?.required ?? false}
            error={error}
            width={field.width}
            containerWidth={containerWidth}
            handle={design?.renderHandle?.(field.key)}
            isSelected={design?.selectedKey === field.key}
            hideLabel={hideLabel}
            isBare={rendersBare(host, field, ui, mode)}
            badge={design?.badges?.[field.key]}
            dropIndicator={dropSideFor(design, field.key)}
            isDragging={design?.draggingKey === field.key}
            onLayoutRect={design?.onCellLayout && (rect => design.onCellLayout?.(field.key, rect))}
            registerMeasure={onCellMeasure && (measure => onCellMeasure(field.key, measure))}
            onPress={mode === 'design' ? () => design?.onSelect?.(field.key) : undefined}
        >
            <FieldInput
                host={host}
                ui={ui}
                field={field}
                control={form.control}
                getValues={form.getValues}
                mode={mode}
                recordId={recordId}
                options={fieldState?.options}
                collab={collab}
                onSubmitEditing={props.onSubmitEditing}
                containerWidth={containerWidth}
                computed={props.computed}
                pendingFiles={props.pendingFiles}
            />
        </FieldCell>
    )
}

function PageFields(props: PageProps) {
    const fields = visibleFieldsOf(props.definition, props.page, props.state, props.mode)
    return (
        <FormLayout>
            {fields.map(f => (
                <Cell key={f.key} {...props} field={f} />
            ))}
        </FormLayout>
    )
}

function PageDescription({ text }: { text?: string }) {
    if (!text) return null
    return <Text className="text-xs text-muted-foreground mt-0.5">{text}</Text>
}

function SectionTitle({ page, isVisible }: { page: FormPage; isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View className="mt-4 mb-2">
            <Text className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                {page.title}
            </Text>
            <PageDescription text={page.description} />
        </View>
    )
}

function PageHeader({
    page,
    isTitleVisible,
    design,
}: PageChromeProps & { isTitleVisible: boolean }) {
    const custom = design?.renderPageHeader?.(page)
    if (custom) return custom
    return <SectionTitle page={page} isVisible={isTitleVisible} />
}

function PageFooter({ page, design }: PageChromeProps) {
    return design?.renderPageFooter?.(page) ?? null
}

/**
 * The bar for a page's end floats in the gap under its last row, before the
 * footer, and takes no height: nothing shifts while it shows.
 */
function PageDropBar({ page, target }: { page: FormPage; target?: DropTarget | null }) {
    if (!target || !('page' in target) || target.page !== page.key) return null
    return (
        <View className="h-0">
            <View
                className="absolute left-0 right-0 -top-2 h-[3px] rounded bg-primary"
                testID={`form-page-${page.key}-drop-end`}
            />
        </View>
    )
}

function DesignPage({ page, isTitleVisible, ...props }: PageProps & { isTitleVisible: boolean }) {
    const { design } = props
    const onPageMeasure = design?.onPageMeasure
    const { ref, onLayout } = useMeasuredRect(
        design?.onPageLayout && (rect => design.onPageLayout?.(page.key, rect)),
        onPageMeasure && (measure => onPageMeasure(page.key, measure))
    )
    return (
        <View ref={ref} onLayout={onLayout} testID={`form-page-${page.key}`}>
            <PageHeader page={page} isTitleVisible={isTitleVisible} design={design} />
            <PageFields {...props} page={page} />
            <PageDropBar page={page} target={design?.draggingKey ? design.dropTarget : null} />
            <PageFooter page={page} design={design} />
        </View>
    )
}

function SubmitErrors({
    form,
    summary = 'all',
    attempted = false,
}: {
    form: UseFormReturn<FormValues>
    summary?: SubmitSummaryKind
    /** A wizard Next that failed counts as an attempt, before any submit. */
    attempted?: boolean
}) {
    const { errors, isSubmitted } = form.formState
    const shown = submitSummary({ errors, isSubmitted, attempted }, summary)
    // The messages name the field by its label already ("Name is required"), so the key is noise.
    return (
        <FormErrorSummary
            errors={shown.errors}
            isEnabled={shown.isEnabled}
            showFieldNames={false}
            testID="form-error-summary"
        />
    )
}

/** The design canvas: one measured block per page, every page shown. */
function DesignSections(props: BodyProps) {
    const pages = props.definition.pages
    return (
        <>
            <SubmitErrors form={props.form} summary={props.summary} />
            {pages.map(page => (
                <DesignPage
                    key={page.key}
                    {...props}
                    page={page}
                    isTitleVisible={pages.length > 1}
                />
            ))}
        </>
    )
}

function Row({
    className,
    testID,
    children,
}: {
    className?: string
    testID?: string
    children: ReactNode
}) {
    return (
        <View className={className} testID={testID}>
            {children}
        </View>
    )
}

type PageRow =
    | { kind: 'title'; key: string }
    | { kind: 'run'; key: string; fields: FormField[] }
    | { kind: 'pinned'; key: string; field: FormField }

/**
 * A page as rows: its title, then its fields in runs that break at every
 * field the host pins a row above. A run is keyed by the pinned field that
 * opens it, so hiding a field never remounts its neighbours' cells. Every key
 * starts with its row's kind, so a field named like a row (`title`) cannot
 * collide with one.
 */
function pageRowsOf(props: BodyProps, page: FormPage, isTitleVisible: boolean): PageRow[] {
    const { definition, state, mode, host, ui } = props
    const rows: PageRow[] = isTitleVisible ? [{ kind: 'title', key: `title:${page.key}` }] : []
    let run: FormField[] = []
    let runKey = `run:${page.key}:^`
    const closeRun = () => {
        if (run.length > 0) rows.push({ kind: 'run', key: runKey, fields: run })
        run = []
    }
    for (const field of visibleFieldsOf(definition, page, state, mode)) {
        if (hasPinnedRow(host, field, ui, mode)) {
            closeRun()
            rows.push({ kind: 'pinned', key: `pinned:${field.key}`, field })
            runKey = `run:${page.key}:${field.key}`
        }
        run.push(field)
    }
    closeRun()
    return rows
}

function pageRowElement(
    props: BodyProps,
    page: FormPage,
    row: PageRow,
    pageTestID: string | undefined
): ReactElement {
    const className = props.scroll?.rowClassName
    if (row.kind === 'pinned') {
        return (
            <Row key={row.key} className={className} testID={`form-pinned-${row.field.key}`}>
                <PinnedHostRow
                    host={props.host}
                    ui={props.ui}
                    field={row.field}
                    control={props.form.control}
                    getValues={props.form.getValues}
                    mode={props.mode}
                    recordId={props.recordId}
                    collab={props.collab}
                />
            </Row>
        )
    }
    if (row.kind === 'title') {
        return (
            <Row key={row.key} className={className} testID={pageTestID}>
                <SectionTitle page={page} isVisible />
            </Row>
        )
    }
    return (
        <Row key={row.key} className={className} testID={pageTestID}>
            <FormLayout>
                {row.fields.map(f => (
                    <Cell key={f.key} {...props} field={f} />
                ))}
            </FormLayout>
        </Row>
    )
}

/**
 * The record layout as one flat list of rows, so that when the renderer owns
 * the scroll view each row is its direct child: `stickyHeaderIndices` pins only
 * a direct child, and a sticky box never leaves its parent. Never a null
 * entry, which would shift the indexes.
 */
function sectionRows(props: BodyProps): { rows: ReactElement[]; pinnedIndexes: number[] } {
    const { definition, state, form } = props
    const pages = definition.pages.filter(p => state.pages[p.key]?.visible)
    // Measured inside the row padding: the width the cells see is the content width.
    const rows: ReactElement[] = [
        <Row key="errors" className={props.scroll?.rowClassName}>
            <View onLayout={props.onContainerLayout}>
                <SubmitErrors form={form} summary={props.summary} />
            </View>
        </Row>,
    ]
    const pinnedIndexes: number[] = []
    for (const page of pages) {
        const pageRows = pageRowsOf(props, page, pages.length > 1)
        // The page's testID rides its first row that is not a pinned one, which carries its own.
        const labelled = pageRows.find(r => r.kind !== 'pinned')
        for (const row of pageRows) {
            if (row.kind === 'pinned') pinnedIndexes.push(rows.length)
            const testID = row === labelled ? `form-page-${page.key}` : undefined
            rows.push(pageRowElement(props, page, row, testID))
        }
    }
    return { rows, pinnedIndexes }
}

function Sections(props: BodyProps) {
    const { rows, pinnedIndexes } = sectionRows(props)
    if (!props.scroll) return <>{rows}</>
    return (
        <ScrollView className="flex-1" stickyHeaderIndices={pinnedIndexes}>
            {rows}
        </ScrollView>
    )
}

function useWizard(props: BodyProps) {
    // The current step and whether Next failed on it: local, synchronous UI
    // state that nothing else needs.
    const [step, setStep] = useState({ index: 0, attemptedNext: false })
    const { definition, state, form, onSubmit } = props
    const pages = definition.pages.filter(p => state.pages[p.key]?.visible)
    const current = Math.min(step.index, pages.length - 1)
    const page = pages[current]
    const isLast = current >= pages.length - 1
    const pageKeys = definition.fields.filter(f => f.page === page?.key).map(f => f.key)
    // Enter on the last page reaches `next` without the footer button's
    // isDisabled, so a fast double press would otherwise submit twice.
    const inFlight = useRef(false)
    const next = async () => {
        if (inFlight.current || form.formState.isSubmitting) return
        inFlight.current = true
        try {
            const ok = await form.trigger(pageKeys)
            if (!ok) {
                setStep({ index: current, attemptedNext: true })
                return
            }
            if (isLast) await form.handleSubmit(() => onSubmit?.())()
            else setStep({ index: current + 1, attemptedNext: false })
        } finally {
            inFlight.current = false
        }
    }
    const back = () => setStep({ index: Math.max(0, current - 1), attemptedNext: false })
    return { pages, page, index: current, isLast, next, back, attemptedNext: step.attemptedNext }
}

function BackButton({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    if (!isVisible) return null
    return (
        <Button variant="outline" onPress={onPress} className="mr-2" testID="form-wizard-back">
            <ButtonText>Back</ButtonText>
        </Button>
    )
}

interface WizardFooterProps {
    isFirst: boolean
    isLast: boolean
    onBack: () => void
    onNext: () => void
    isSubmitting: boolean
    submitLabel: string
}

function WizardFooter(props: WizardFooterProps) {
    const { isFirst, isLast, onBack, onNext, isSubmitting, submitLabel } = props
    return (
        <View className="flex-row justify-end mt-2">
            <BackButton isVisible={!isFirst} onPress={onBack} />
            <Button onPress={onNext} isDisabled={isSubmitting} testID="form-wizard-next">
                <ButtonText>{isLast ? submitLabel : 'Next'}</ButtonText>
            </Button>
        </View>
    )
}

function Wizard(props: BodyProps) {
    const w = useWizard(props)
    const { formState } = props.form
    if (!w.page) return null
    return (
        <View>
            <WizardSteps pages={w.pages} index={w.index} />
            <SubmitErrors form={props.form} summary={props.summary} attempted={w.attemptedNext} />
            <PageFields {...props} page={w.page} onSubmitEditing={w.next} />
            <WizardFooter
                isFirst={w.index === 0}
                isLast={w.isLast}
                onBack={w.back}
                onNext={w.next}
                isSubmitting={formState.isSubmitting}
                submitLabel={props.submitLabel ?? 'Create'}
            />
        </View>
    )
}

function modeFor(layout: FormLayoutKind, readOnly?: boolean, recordId?: string): FieldMode {
    if (layout === 'design') return 'design'
    if (readOnly) return 'read'
    return recordId ? 'edit' : 'create'
}

function Body(props: BodyProps) {
    if (props.layout === 'wizard') return <Wizard {...props} />
    if (props.layout === 'design') return <DesignSections {...props} />
    return <Sections {...props} />
}

/** The sections layout measures inside its row padding (see sectionRows); the others measure the root. */
function rootLayoutFor(props: FormRendererProps, onLayout: (e: LayoutChangeEvent) => void) {
    if (props.layout !== 'sections') return { onLayout, className: undefined }
    return { onLayout: undefined, className: props.scroll ? 'flex-1' : undefined }
}

export function FormRenderer(props: FormRendererProps) {
    const { definition, host, form, context, layout, readOnly, recordId, schemaRef } = props
    const { width, onLayout } = useContainerWidth()
    const mode = modeFor(layout, readOnly, recordId)
    const isEditable = mode === 'create' || mode === 'edit'
    const { state, schema, computed } = useFormState(
        definition,
        host,
        form.control,
        context,
        isEditable ? form.setValue : undefined,
        props.ui?.schemas ?? NO_SCHEMAS,
        props.ui?.recordSchema
    )
    // The resolver reads this ref, so required-if rules validate against the live state.
    if (schemaRef) schemaRef.current = schema
    const root = rootLayoutFor(props, onLayout)
    return (
        <View
            onLayout={root.onLayout}
            className={root.className}
            testID={props.testID ?? 'form-renderer'}
        >
            <Body
                {...props}
                state={state}
                computed={computed}
                containerWidth={width}
                mode={mode}
                onContainerLayout={onLayout}
            />
        </View>
    )
}
