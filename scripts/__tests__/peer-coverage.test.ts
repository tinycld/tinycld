import * as fs from 'node:fs'
import * as path from 'node:path'
import { describe, expect, it } from 'vitest'
import { getPackages } from '../../../tinycld.packages'
import { APP_DIR, memberDir } from '../paths'
import { findUncoveredPeers } from '../peer-coverage'

const readJson = (file: string) => JSON.parse(fs.readFileSync(file, 'utf8'))

describe('findUncoveredPeers', () => {
    const acme = {
        name: '@acme/widgets',
        peerDependencies: {
            '@tinycld/core': '*',
            react: '*',
            'left-pad': '^1.3.0',
            'acme-charts': '^2.0.0',
        },
    }

    it('reports a non-@tinycld peer the shell and the pins both miss', () => {
        expect(findUncoveredPeers([acme], new Set(['left-pad']), new Set(['react']))).toEqual([
            { member: '@acme/widgets', peer: 'acme-charts' },
        ])
    })

    it('accepts a member with no peers', () => {
        expect(findUncoveredPeers([{ name: '@acme/empty' }], new Set(), new Set())).toEqual([])
    })
})

describe('installed member peers', () => {
    it('are each declared by the app shell or pinned by core', () => {
        const shell = readJson(path.join(APP_DIR, 'package.json'))
        const shellDeps = new Set([
            ...Object.keys(shell.dependencies ?? {}),
            ...Object.keys(shell.devDependencies ?? {}),
        ])
        const pinned = new Set(
            Object.keys(readJson(path.join(APP_DIR, 'core', 'package-versions.json'))).filter(
                key => key !== '//'
            )
        )
        const members = getPackages().map(name =>
            readJson(path.join(memberDir(name), 'package.json'))
        )
        expect(findUncoveredPeers(members, shellDeps, pinned)).toEqual([])
    })
})
