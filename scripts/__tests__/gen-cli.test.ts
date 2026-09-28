import { describe, expect, it } from 'vitest'
import {
    buildCliExtensionsSource,
    buildCliGoWork,
    buildMemberCliGoWork,
    buildSearchSlugsSource,
    type CliPkg,
} from '../gen-cli'

const gizmos: CliPkg = {
    slug: 'gizmos',
    module: 'tinycld.org/packages/gizmos/cli',
    cliRelPath: '../../gizmos/cli',
}

describe('buildCliExtensionsSource', () => {
    it('emits a no-op that still references cobra and the client package', () => {
        const go = buildCliExtensionsSource([])
        expect(go).toContain('func registerPackageCommands(_ *cobra.Command, _ *client.Client) {}')
        expect(go).toContain('"github.com/spf13/cobra"')
        expect(go).toContain('"tinycld.org/cli/client"')
    })

    it('imports + registers each cli package by slug identifier', () => {
        const go = buildCliExtensionsSource([gizmos])
        expect(go).toContain('gizmos "tinycld.org/packages/gizmos/cli"')
        expect(go).toContain('gizmos.Register(root, c)')
        expect(go).toContain('func registerPackageCommands(root *cobra.Command, c *client.Client)')
    })

    it('camelizes hyphenated slugs into valid Go identifiers', () => {
        const go = buildCliExtensionsSource([
            {
                slug: 'gizmo-import',
                module: 'tinycld.org/packages/gizmo-import/cli',
                cliRelPath: '../../gizmo-import/cli',
            },
        ])
        expect(go).toContain('gizmoImport "tinycld.org/packages/gizmo-import/cli"')
        expect(go).toContain('gizmoImport.Register(root, c)')
    })

    it('rejects a module path that would break out of the generated Go import', () => {
        const bad: CliPkg = { ...gizmos, module: 'tinycld.org/x"; evil()//' }
        expect(() => buildCliExtensionsSource([bad])).toThrow(/unsafe value/)
    })
})

describe('buildCliGoWork', () => {
    it('includes ., the format module, and each cli package use', () => {
        const work = buildCliGoWork([gizmos])
        expect(work).toContain('use (')
        expect(work).toContain('    .')
        expect(work).toContain('    ../../gizmos/cli')
    })

    // The CLI reads backup archives, so it uses core's nested archive-format
    // module — but NOT core proper, whose graph drags in the PocketBase fork.
    it('uses the nested format module and never core proper', () => {
        const work = buildCliGoWork([gizmos])
        expect(work).toContain('    ../core/server/backup/format')
        expect(work).not.toContain('    ../core/server\n')
    })

    // Members require tinycld.org/cli v0.0.0; without a versioned workspace
    // replace the graph load hits the proxy and fails. Unversioned is rejected
    // ("replaced at all versions") because the module is itself a `use` member.
    it('replaces tinycld.org/cli (versioned) when members are present', () => {
        const work = buildCliGoWork([gizmos])
        expect(work).toContain('replace tinycld.org/cli v0.0.0 => .')
    })

    it('is a valid single-module workspace when no package declares cli', () => {
        const work = buildCliGoWork([])
        expect(work).toContain('use (')
        expect(work).toContain('    .')
        // still the format module — the CLI itself requires it on every assembly
        expect(work).toContain('    ../core/server/backup/format')
        expect(work).not.toContain('replace')
    })
})

describe('buildSearchSlugsSource', () => {
    it('emits the slugs of packages declaring search, sorted', () => {
        const go = buildSearchSlugsSource(['gizmos', 'gadgets', 'cogs'])
        expect(go).toContain('var searchSlugs = []string{"cogs", "gadgets", "gizmos"}')
    })

    // The search set is NOT the cli set: boards and contacts contribute a search
    // source but ship no CLI commands, so deriving one list from the other
    // would make `cards:` parse as a literal word in the terminal.
    it('emits an empty slice when no package declares search', () => {
        const go = buildSearchSlugsSource([])
        expect(go).toContain('var searchSlugs = []string{}')
    })

    it('rejects a slug that would break out of the generated Go literal', () => {
        expect(() => buildSearchSlugsSource(['gizmos"; evil()//'])).toThrow(/unsafe value/)
    })
})

describe('buildMemberCliGoWork', () => {
    it('replaces tinycld.org/cli so a standalone member build resolves it', () => {
        const work = buildMemberCliGoWork('../../tinycld/cli')
        expect(work).toContain('use .')
        expect(work).toContain('replace tinycld.org/cli => ../../tinycld/cli')
    })

    // tinycld.org/cli requires the format module at v0.0.0, so a standalone
    // member cli build needs it replaced too — versioned, and with the path
    // normalized (never `.../cli/../core/...`).
    it('replaces the format module at a normalized path', () => {
        const work = buildMemberCliGoWork('../../tinycld/cli')
        expect(work).toContain(
            'replace tinycld.org/core/backup/format v0.0.0 => ../../tinycld/core/server/backup/format'
        )
    })
})
