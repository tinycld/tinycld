import * as fs from 'node:fs'
import * as path from 'node:path'
import { describe, expect, it } from 'vitest'

const APP_DIR = path.resolve(__dirname, '..', '..')

const readVersion = (...segments: string[]) =>
    JSON.parse(fs.readFileSync(path.join(APP_DIR, ...segments), 'utf8')).version

describe('@tinycld/core version', () => {
    // Features pin `peerVersions['@tinycld/core']` to the released (app shell)
    // version, but the compatibility solver compares that range against the
    // nested core/package.json. When the two differ, no feature's range can match
    // and no package set resolves. The release script bumps both; this catches a
    // hand edit that bumps only one.
    it('matches the app shell version', () => {
        expect(readVersion('core', 'package.json')).toBe(readVersion('package.json'))
    })
})
