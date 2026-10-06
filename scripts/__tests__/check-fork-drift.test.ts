import { describe, expect, it } from 'vitest'
import { findGoForks, findNpmForks, staleForks } from '../check-fork-drift'

const TODAY = new Date('2026-10-06T00:00:00Z')

describe('findGoForks', () => {
    it('finds a replace that points at another module path', () => {
        const forks = findGoForks({
            'core/server/go.mod':
                'module tinycld.org/core\n\nreplace github.com/emersion/go-webdav => github.com/nathanstitt/go-webdav v0.7.1\n',
        })
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
        const forks = findGoForks({
            'server/go.mod':
                'replace github.com/pocketbase/pocketbase => ../third_party/pocketbase\n',
        })
        expect(forks.map(f => f.name)).toEqual(['github.com/pocketbase/pocketbase'])
        expect(forks[0].pinnedAt).toBe('../third_party/pocketbase')
    })

    it('ignores a replace between our own modules, which has no upstream', () => {
        const forks = findGoForks({
            'server/go.mod': 'replace tinycld.org/core => ../core/server\n',
        })
        expect(forks).toEqual([])
    })

    it('reports a fork once even when several modules replace it', () => {
        const forks = findGoForks({
            'server/go.mod':
                'replace github.com/osshield/gopbs => github.com/nathanstitt/gopbs v1.0.0\n',
            'core/server/go.mod':
                'replace github.com/osshield/gopbs => github.com/nathanstitt/gopbs v1.0.0\n',
        })
        expect(forks).toHaveLength(1)
    })
})

describe('findNpmForks', () => {
    it('finds a github: pin and ignores ordinary semver pins', () => {
        const forks = findNpmForks(
            JSON.stringify({
                '//': 'a comment field',
                react: '19.2.0',
                'react-native-drax': 'github:nathanstitt/react-native-drax#bc83061',
            })
        )
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
        expect(findNpmForks(JSON.stringify({ react: '19.2.0', expo: '55.0.26' }))).toEqual([])
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
