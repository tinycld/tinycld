import { fireEvent } from '@testing-library/react'
import { type UseFormReturn, useForm } from 'react-hook-form'
import type { FormDefinition } from '../../../lib/forms/definition'
import type { FormHostUi } from '../../../lib/forms/host-ui'
import { evaluate, type FormValues } from '../../../lib/forms/logic'
import { buildSchema } from '../../../lib/forms/values'
import type { FormHost } from '../../../lib/packages/derive-form-hosts'
import { OverlayProvider } from '../../overlay'
import { FormRenderer } from '../FormRenderer'
import { useSchemaRefResolver } from '../use-schema-ref-resolver'

export const ctx = { today: '2026-01-01', userId: 'u' }

export interface TableHarnessProps {
    host: FormHost
    definition: FormDefinition
    initialValues: FormValues
    layout?: 'sections' | 'wizard'
    ui?: FormHostUi | null
    readOnly?: boolean
    recordId?: string
    onSubmit?: () => void
    formRef?: { current: UseFormReturn<FormValues> | null }
}

/**
 * The renderer over a plain react-hook-form, as a create screen would mount it.
 * Row menus open in the overlay host, as they do under the app's root provider.
 */
export function TableHarness(props: TableHarnessProps) {
    const { host, definition, initialValues, formRef } = props
    const initial = buildSchema(definition, host, evaluate(definition, initialValues, ctx))
    const { schemaRef, resolver } = useSchemaRefResolver(initial)
    const form = useForm<FormValues>({ defaultValues: initialValues, resolver })
    if (formRef) formRef.current = form
    return (
        <OverlayProvider>
            <FormRenderer
                host={host}
                ui={props.ui ?? null}
                definition={definition}
                layout={props.layout ?? 'sections'}
                form={form}
                schemaRef={schemaRef}
                context={ctx}
                readOnly={props.readOnly}
                recordId={props.recordId}
                onSubmit={props.onSubmit}
            />
        </OverlayProvider>
    )
}

export function byTestId(container: HTMLElement, id: string): HTMLElement {
    const el = container.querySelector<HTMLElement>(`[testID="${id}"]`)
    if (!el) throw new Error(`no element with testID ${id}`)
    return el
}

export function rowIdsOf(container: HTMLElement, table: string): string[] {
    const prefix = `form-table-${table}-row-`
    return [...container.querySelectorAll(`[testID^="${prefix}"]`)].map(el =>
        (el.getAttribute('testID') ?? '').slice(prefix.length)
    )
}

/** A row menu item; the menu renders in the overlay host, outside the container. */
export function menuItem(table: string, action: string): HTMLElement {
    const id = `form-table-${table}-menu-${action}`
    const el = document.body.querySelector<HTMLElement>(`[data-testid="${id}"]`)
    if (!el) throw new Error(`no menu item ${id}`)
    return el
}

/** Opens a row's menu (its one row button) and picks an action: `up`, `down`, `remove`, … */
export function pickRowAction(
    container: HTMLElement,
    table: string,
    rowId: string,
    action: string
) {
    fireEvent.click(byTestId(container, `form-table-${table}-menu-${rowId}`))
    fireEvent.click(menuItem(table, action))
}
