import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { loadAutomationDefs, mergeAutomationDefs } from '../gen-automation'

const tmpDirs: string[] = []
afterEach(() => {
    for (const d of tmpDirs.splice(0)) fs.rmSync(d, { recursive: true, force: true })
})

function makePkg(exportsMap: Record<string, string>, files: Record<string, string>): string {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'gen-automation-'))
    tmpDirs.push(dir)
    fs.writeFileSync(
        path.join(dir, 'package.json'),
        JSON.stringify({ name: '@tinycld/fake', exports: exportsMap })
    )
    for (const [rel, content] of Object.entries(files)) {
        const abs = path.join(dir, rel)
        fs.mkdirSync(path.dirname(abs), { recursive: true })
        fs.writeFileSync(abs, content)
    }
    return dir
}

describe('loadAutomationDefs', () => {
    it('resolves the subpath through the exports map and imports the module', async () => {
        // .js fixture on purpose: vitest's transform pipeline doesn't cover a
        // dynamic import of a bare .ts file in tmpdir. Production targets are
        // .ts and load fine because the generator runs under tsx.
        const dir = makePkg(
            { './automation': './tinycld/fake/automation.js' },
            {
                'tinycld/fake/automation.js':
                    "export default { triggers: [{ id: 'thing-created', label: 'A thing is created', collection: 'fake_things', on: 'create' }] }\n",
            }
        )
        const defs = await loadAutomationDefs(dir, '@tinycld/fake', 'automation')
        expect(defs.triggers?.[0]?.id).toBe('thing-created')
    })

    it('throws a clear error when the exports map lacks the subpath', async () => {
        const dir = makePkg({}, {})
        await expect(loadAutomationDefs(dir, '@tinycld/fake', 'automation')).rejects.toThrow(
            /no '\.\/automation' entry/
        )
    })

    it('throws when the module has no default export', async () => {
        const dir = makePkg(
            { './automation': './tinycld/fake/automation.js' },
            { 'tinycld/fake/automation.js': 'export const x = 1\n' }
        )
        await expect(loadAutomationDefs(dir, '@tinycld/fake', 'automation')).rejects.toThrow(
            /default-export/i
        )
    })

    it('throws naming the entry when the exports map value is a conditional-exports object', async () => {
        const dir = makePkg({}, {})
        // makePkg only accepts string exports values; overwrite package.json
        // directly to exercise the conditional-exports (object entry) shape.
        fs.writeFileSync(
            path.join(dir, 'package.json'),
            JSON.stringify({
                name: '@tinycld/fake',
                exports: { './automation': { import: './tinycld/fake/automation.js' } },
            })
        )
        await expect(loadAutomationDefs(dir, '@tinycld/fake', 'automation')).rejects.toThrow(
            /'\.\/automation'.*not a plain string/
        )
    })
})

describe('mergeAutomationDefs', () => {
    it('puts core first and validates every package', () => {
        const merged = mergeAutomationDefs([
            {
                slug: 'mail',
                defs: {
                    triggers: [
                        {
                            id: 'message-received',
                            label: 'A message arrives',
                            collection: 'mail_messages',
                            on: 'create',
                        },
                    ],
                },
            },
        ])
        expect(merged.packages[0].slug).toBe('core')
        // The assertion is the ORDER — core first — not core's catalog, which
        // grows. Pin the two synthetic triggers that make core special (a
        // feature package may not declare them; see the test below) rather
        // than the whole list.
        expect(merged.packages[0].triggers.map(t => t.id)).toEqual(
            expect.arrayContaining(['schedule', 'manual'])
        )
        expect(merged.packages[1].slug).toBe('mail')
    })

    it('throws when a feature declares a synthetic trigger', () => {
        expect(() =>
            mergeAutomationDefs([
                {
                    slug: 'mail',
                    defs: { triggers: [{ id: 'x', label: 'x', synthetic: 'schedule' }] },
                },
            ])
        ).toThrow(/synthetic/)
    })
})
