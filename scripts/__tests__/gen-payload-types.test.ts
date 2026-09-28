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

const gizmos: PayloadFeatureInput = {
    name: '@tinycld/gizmos',
    dir: '/ws/gizmos',
    manifest: { slug: 'gizmos', payloads: { package: 'server/api' } },
}

const doodads: PayloadFeatureInput = {
    name: '@tinycld/doodads',
    dir: '/ws/doodads',
    manifest: { slug: 'doodads' },
}

describe('planPayloadEmits', () => {
    it('plans one emit per feature with a payloads block', () => {
        const emits = planPayloadEmits([gizmos, doodads], GENERATED)
        expect(emits).toEqual([
            {
                slug: 'gizmos',
                srcDir: path.join('/ws/gizmos', 'server/api'),
                outFile: path.join(GENERATED, 'gizmos-api.ts'),
            },
        ])
    })

    it('skips features without a payloads block', () => {
        expect(planPayloadEmits([doodads], GENERATED)).toEqual([])
    })

    it('rejects unsafe payloads.package values', () => {
        const quoted: PayloadFeatureInput = {
            ...gizmos,
            manifest: { slug: 'gizmos', payloads: { package: `server'; evil()//` } },
        }
        expect(() => planPayloadEmits([quoted], GENERATED)).toThrow(/unsafe value/)
    })

    it('rejects path traversal in payloads.package', () => {
        const traversal: PayloadFeatureInput = {
            ...gizmos,
            manifest: { slug: 'gizmos', payloads: { package: '../outside' } },
        }
        expect(() => planPayloadEmits([traversal], GENERATED)).toThrow(/relative path inside/)
    })

    it('rejects an unsafe slug', () => {
        const badSlug: PayloadFeatureInput = {
            ...gizmos,
            manifest: { slug: 'gizmos/../..', payloads: { package: 'server/api' } },
        }
        expect(() => planPayloadEmits([badSlug], GENERATED)).toThrow(/invalid slug/)
    })
})

describe('orphanPayloadFiles', () => {
    it('returns only -api.ts files whose slug is absent, tolerating a missing dir', () => {
        const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'gen-payload-types-'))
        try {
            for (const f of ['gizmos-api.ts', 'cogs-api.ts', 'package-icons.ts', 'notes.txt']) {
                fs.writeFileSync(path.join(dir, f), '')
            }
            const orphans = orphanPayloadFiles(dir, new Set(['gizmos']))
            expect(orphans).toEqual([path.join(dir, 'cogs-api.ts')])
        } finally {
            fs.rmSync(dir, { recursive: true, force: true })
        }
        expect(orphanPayloadFiles('/nonexistent/dir', new Set())).toEqual([])
    })
})
