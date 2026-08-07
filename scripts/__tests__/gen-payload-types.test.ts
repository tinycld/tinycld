import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { describe, expect, it } from 'vitest'
import {
    orphanPayloadFiles,
    type PayloadFeatureInput,
    planPayloadEmits,
} from '../gen-payload-types'

const GENERATED = '/app/lib/generated'

const mail: PayloadFeatureInput = {
    name: '@tinycld/mail',
    dir: '/ws/mail',
    manifest: { slug: 'mail', payloads: { package: 'server/api' } },
}

const contacts: PayloadFeatureInput = {
    name: '@tinycld/contacts',
    dir: '/ws/contacts',
    manifest: { slug: 'contacts' },
}

describe('planPayloadEmits', () => {
    it('plans one emit per feature with a payloads block', () => {
        const emits = planPayloadEmits([mail, contacts], GENERATED)
        expect(emits).toEqual([
            {
                slug: 'mail',
                srcDir: path.join('/ws/mail', 'server/api'),
                outFile: path.join(GENERATED, 'mail-api.ts'),
            },
        ])
    })

    it('skips features without a payloads block', () => {
        expect(planPayloadEmits([contacts], GENERATED)).toEqual([])
    })

    it('rejects unsafe payloads.package values', () => {
        const quoted: PayloadFeatureInput = {
            ...mail,
            manifest: { slug: 'mail', payloads: { package: `server'; evil()//` } },
        }
        expect(() => planPayloadEmits([quoted], GENERATED)).toThrow(/unsafe value/)
    })

    it('rejects path traversal in payloads.package', () => {
        const traversal: PayloadFeatureInput = {
            ...mail,
            manifest: { slug: 'mail', payloads: { package: '../outside' } },
        }
        expect(() => planPayloadEmits([traversal], GENERATED)).toThrow(/relative path inside/)
    })

    it('rejects an unsafe slug', () => {
        const badSlug: PayloadFeatureInput = {
            ...mail,
            manifest: { slug: 'mail/../..', payloads: { package: 'server/api' } },
        }
        expect(() => planPayloadEmits([badSlug], GENERATED)).toThrow(/invalid slug/)
    })
})

describe('orphanPayloadFiles', () => {
    it('returns only -api.ts files whose slug is absent, tolerating a missing dir', () => {
        const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'gen-payload-types-'))
        try {
            for (const f of ['mail-api.ts', 'drive-api.ts', 'package-icons.ts', 'notes.txt']) {
                fs.writeFileSync(path.join(dir, f), '')
            }
            const orphans = orphanPayloadFiles(dir, new Set(['mail']))
            expect(orphans).toEqual([path.join(dir, 'drive-api.ts')])
        } finally {
            fs.rmSync(dir, { recursive: true, force: true })
        }
        expect(orphanPayloadFiles('/nonexistent/dir', new Set())).toEqual([])
    })
})
