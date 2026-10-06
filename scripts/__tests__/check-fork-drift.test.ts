import { describe, expect, it } from 'vitest'
import { findGoForks, findNpmForks, staleForks } from '../check-fork-drift'

const TODAY = new Date('2026-10-06T00:00:00Z')

describe('findGoForks', () => {
    it('finds a replace that points at another module path', () => {
        const { forks, errors } = findGoForks({
            'core/server/go.mod':
                'module tinycld.org/core\n\nreplace github.com/emersion/go-webdav => github.com/nathanstitt/go-webdav v0.7.1\n',
        })
        expect(errors).toEqual([])
        expect(forks).toEqual([
            {
                name: 'github.com/emersion/go-webdav',
                upstream: 'github.com/emersion/go-webdav',
                pinnedAt: 'github.com/nathanstitt/go-webdav v0.7.1',
                kind: 'go',
            },
        ])
    })

    it('finds a replace that points at a vendored local directory', () => {
        const { forks } = findGoForks({
            'server/go.mod':
                'replace github.com/pocketbase/pocketbase => ../third_party/pocketbase\n',
        })
        expect(forks.map(f => f.name)).toEqual(['github.com/pocketbase/pocketbase'])
        expect(forks[0].pinnedAt).toBe('../third_party/pocketbase')
    })

    it('ignores a replace between our own modules, which has no upstream', () => {
        const { forks, errors } = findGoForks({
            'server/go.mod': 'replace tinycld.org/core => ../core/server\n',
        })
        expect(forks).toEqual([])
        expect(errors).toEqual([])
    })

    it('reports a fork once even when several modules replace it', () => {
        const { forks } = findGoForks({
            'server/go.mod':
                'replace github.com/osshield/gopbs => github.com/nathanstitt/gopbs v1.0.0\n',
            'core/server/go.mod':
                'replace github.com/osshield/gopbs => github.com/nathanstitt/gopbs v1.0.0\n',
        })
        expect(forks).toHaveLength(1)
    })

    it('finds a replace written in block form', () => {
        const { forks, errors } = findGoForks({
            'server/go.mod':
                'replace (\n\tgithub.com/a/b => github.com/c/d v1.0.0\n\tgithub.com/e/f => github.com/g/h v2.0.0\n)\n',
        })
        expect(errors).toEqual([])
        expect(forks.map(f => f.name).sort()).toEqual(['github.com/a/b', 'github.com/e/f'])
        expect(forks.find(f => f.name === 'github.com/a/b')?.pinnedAt).toBe('github.com/c/d v1.0.0')
    })

    it('finds a replace with the version on the left-hand side', () => {
        const { forks, errors } = findGoForks({
            'server/go.mod': 'replace github.com/a/b v1.0.0 => github.com/c/d v2.0.0\n',
        })
        expect(errors).toEqual([])
        expect(forks).toEqual([
            {
                name: 'github.com/a/b',
                upstream: 'github.com/a/b',
                pinnedAt: 'github.com/c/d v2.0.0',
                kind: 'go',
            },
        ])
    })

    it('strips a trailing comment from the replace target', () => {
        const { forks } = findGoForks({
            'server/go.mod':
                'replace github.com/a/b => github.com/c/d v1.0.0 // fork, see HANDOFF\n',
        })
        expect(forks[0].pinnedAt).toBe('github.com/c/d v1.0.0')
    })

    it('reports an unparseable replace line as an error instead of skipping it', () => {
        const { forks, errors } = findGoForks({
            'server/go.mod': 'replace this is not valid go.mod syntax\n',
        })
        expect(forks).toEqual([])
        expect(errors).toHaveLength(1)
        expect(errors[0]).toContain('server/go.mod')
    })

    it('reports an error when a replace ( block is never closed with )', () => {
        const { forks, errors } = findGoForks({
            'server/go.mod': 'replace (\n\tgithub.com/a => github.com/b v1.0.0\n',
        })
        // The fork inside the block is still found...
        expect(forks.map(f => f.name)).toEqual(['github.com/a'])
        // ...but the missing close paren must not go unreported.
        expect(errors).toHaveLength(1)
        expect(errors[0]).toContain('server/go.mod')
        expect(errors[0]).toMatch(/never closed/i)
    })
})

describe('findNpmForks', () => {
    it('finds a github: pin and ignores ordinary semver pins', () => {
        const { forks, errors } = findNpmForks(
            JSON.stringify({
                '//': 'a comment field',
                react: '19.2.0',
                'react-native-drax': 'github:nathanstitt/react-native-drax#bc83061',
            })
        )
        expect(errors).toEqual([])
        expect(forks).toEqual([
            {
                name: 'react-native-drax',
                upstream: 'nathanstitt/react-native-drax',
                pinnedAt: 'bc83061',
                kind: 'npm',
            },
        ])
    })

    it('returns nothing when every pin is ordinary semver', () => {
        const { forks, errors } = findNpmForks(JSON.stringify({ react: '19.2.0', expo: '55.0.26' }))
        expect(forks).toEqual([])
        expect(errors).toEqual([])
    })

    it('finds git+https, git+ssh and bare owner/repo shorthand pins', () => {
        const { forks, errors } = findNpmForks(
            JSON.stringify({
                a: 'git+https://github.com/x/y.git#abc123',
                b: 'git+ssh://git@github.com/x/z.git#def456',
                c: 'owner/repo#ghi789',
            })
        )
        expect(errors).toEqual([])
        expect(forks.map(f => f.name).sort()).toEqual(['a', 'b', 'c'])
    })

    it('reports an unrecognised pin as an error instead of dropping it', () => {
        const { forks, errors } = findNpmForks(JSON.stringify({ weird: 'not-a-known-pin-form!!' }))
        expect(forks).toEqual([])
        expect(errors).toHaveLength(1)
        expect(errors[0]).toContain('weird')
    })

    it('still recognises the real drax fork pin', () => {
        const { forks, errors } = findNpmForks(
            JSON.stringify({
                'react-native-drax': 'github:nathanstitt/react-native-drax#bc83061',
            })
        )
        expect(errors).toEqual([])
        expect(forks).toEqual([
            {
                name: 'react-native-drax',
                upstream: 'nathanstitt/react-native-drax',
                pinnedAt: 'bc83061',
                kind: 'npm',
            },
        ])
    })

    it('does not mistake a path-like value for a bare owner/repo shorthand', () => {
        const { forks, errors } = findNpmForks(JSON.stringify({ someDep: 'dist/index.js' }))
        expect(forks).toEqual([])
        expect(errors).toHaveLength(1)
        expect(errors[0]).toContain('someDep')
    })

    it('silently ignores workspace:, catalog:, and plain dist-tags', () => {
        const { forks, errors } = findNpmForks(
            JSON.stringify({
                a: 'workspace:*',
                b: 'catalog:',
                c: 'latest',
                d: 'next',
            })
        )
        expect(forks).toEqual([])
        expect(errors).toEqual([])
    })

    it('still errors on an unknown bare shorthand-shaped form outside those exceptions', () => {
        const { forks, errors } = findNpmForks(JSON.stringify({ weird: 'not@valid#anything' }))
        expect(forks).toEqual([])
        expect(errors).toHaveLength(1)
    })
})

describe('readReviewWindows errors', () => {
    // Exercised through staleForks' inputs: the parsing is covered by the
    // integration run in Step 6, but the contract that a bad file REPORTS
    // rather than silently yields no windows is asserted here.
    it('treats a fork with no window as unreviewed, which fails the job', () => {
        const result = staleForks(
            [{ name: 'f', upstream: 'u', pinnedAt: 'p', kind: 'go' as const }],
            [],
            TODAY
        )
        expect(result.unreviewed).toEqual(['f'])
    })
})

describe('staleForks', () => {
    const fork = {
        name: 'github.com/pocketbase/pocketbase',
        upstream: 'x',
        pinnedAt: 'y',
        kind: 'go' as const,
    }

    it('passes a fork reviewed inside its window', () => {
        const result = staleForks(
            [fork],
            [{ fork: fork.name, reviewed: '2026-09-15', days: 90 }],
            TODAY
        )
        expect(result).toEqual({ stale: [], unreviewed: [] })
    })

    it('flags a fork whose review window has passed', () => {
        const result = staleForks(
            [fork],
            [{ fork: fork.name, reviewed: '2026-01-01', days: 90 }],
            TODAY
        )
        expect(result.stale).toEqual([fork.name])
    })

    it('flags a fork with no review window at all', () => {
        const result = staleForks([fork], [], TODAY)
        expect(result.unreviewed).toEqual([fork.name])
    })
})
