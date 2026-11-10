import { describe, expect, it } from 'vitest'
import { columnSchema, definitionSchema, hostColumnSchema } from '../definition'
import { findBuiltin, hostDefSchema } from '../host'
import { orderHost, orderHostInput } from './table-fixtures'

describe('host table columns', () => {
    it('keeps the component a host column names', () => {
        expect(findBuiltin(orderHost, 'lines')?.columns?.[0]).toMatchObject({
            key: 'item',
            component: 'ItemCell',
            required: true,
        })
    })

    it('strips a component from a custom table column', () => {
        const def = definitionSchema.parse({
            version: 1,
            pages: [{ key: 'main', title: 'Main' }],
            fields: [
                {
                    key: 'parts',
                    kind: 'table',
                    page: 'main',
                    label: 'Parts',
                    columns: [
                        { key: 'label', kind: 'text', label: 'Label', component: 'ItemCell' },
                    ],
                },
            ],
        })
        const field = def.fields[0]
        expect(field.kind).toBe('table')
        expect(field.kind === 'table' && 'component' in field.columns[0]).toBe(false)
    })

    describe('flex weight', () => {
        const text = { key: 'description', kind: 'text', label: 'Description' }

        it('keeps a flex weight on a host column', () => {
            const parsed = hostColumnSchema.parse({ ...text, flex: 3 })
            expect(parsed).toMatchObject({ key: 'description', flex: 3 })
        })

        it('keeps a host column without a flex weight valid', () => {
            const parsed = hostColumnSchema.parse(text)
            expect(parsed.key).toBe('description')
            expect('flex' in parsed).toBe(false)
        })

        it.each([0, -1, 13])('refuses a flex weight of %d', flex => {
            expect(hostColumnSchema.safeParse({ ...text, flex }).success).toBe(false)
        })

        it('does not carry flex through a custom admin column', () => {
            const parsed = columnSchema.parse({ ...text, flex: 3 })
            expect('flex' in parsed).toBe(false)
        })
    })

    describe('minimum width', () => {
        const text = { key: 'qty', kind: 'number', label: 'Qty' }

        it('keeps a minWidth on a host column without a flex weight', () => {
            const parsed = hostColumnSchema.parse({ ...text, minWidth: 72 })
            expect(parsed).toMatchObject({ key: 'qty', minWidth: 72 })
        })

        it('keeps a minWidth beside a flex weight', () => {
            const parsed = hostColumnSchema.parse({ ...text, minWidth: 72, flex: 0.5 })
            expect(parsed).toMatchObject({ key: 'qty', minWidth: 72, flex: 0.5 })
        })

        it('keeps a host column without a minWidth valid', () => {
            const parsed = hostColumnSchema.parse(text)
            expect(parsed.key).toBe('qty')
            expect('minWidth' in parsed).toBe(false)
        })

        it.each([0, -1, 401, 72.5])('refuses a minWidth of %s', minWidth => {
            expect(hostColumnSchema.safeParse({ ...text, minWidth }).success).toBe(false)
        })

        it('does not carry minWidth through a custom admin column', () => {
            const parsed = columnSchema.parse({ ...text, minWidth: 72 })
            expect('minWidth' in parsed).toBe(false)
        })
    })

    it('still rejects a host column of an unknown kind', () => {
        const bad = {
            ...orderHostInput,
            builtin: [
                {
                    key: 'lines',
                    label: 'Lines',
                    kind: 'table',
                    storage: 'rows',
                    columns: [{ key: 'blob', kind: 'blob', label: 'Blob' }],
                },
            ],
        }
        expect(hostDefSchema.safeParse(bad).success).toBe(false)
    })
})
