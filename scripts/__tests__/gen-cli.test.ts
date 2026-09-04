import { describe, expect, it } from 'vitest'
import {
    buildCliExtensionsSource,
    buildCliGoWork,
    buildMemberCliGoWork,
    buildSearchSlugsSource,
    type CliPkg,
} from '../gen-cli'

const mail: CliPkg = {
    slug: 'mail',
    module: 'tinycld.org/packages/mail/cli',
    cliRelPath: '../../mail/cli',
}

describe('buildCliExtensionsSource', () => {
    it('emits a no-op that still references cobra and the client package', () => {
        const go = buildCliExtensionsSource([])
        expect(go).toContain('func registerPackageCommands(_ *cobra.Command, _ *client.Client) {}')
        expect(go).toContain('"github.com/spf13/cobra"')
        expect(go).toContain('"tinycld.org/cli/client"')
    })

    it('imports + registers each cli package by slug identifier', () => {
        const go = buildCliExtensionsSource([mail])
        expect(go).toContain('mail "tinycld.org/packages/mail/cli"')
        expect(go).toContain('mail.Register(root, c)')
        expect(go).toContain('func registerPackageCommands(root *cobra.Command, c *client.Client)')
    })

    it('camelizes hyphenated slugs into valid Go identifiers', () => {
        const go = buildCliExtensionsSource([
            {
                slug: 'google-takeout-import',
                module: 'tinycld.org/packages/google-takeout-import/cli',
                cliRelPath: '../../google-takeout-import/cli',
            },
        ])
        expect(go).toContain('googleTakeoutImport "tinycld.org/packages/google-takeout-import/cli"')
        expect(go).toContain('googleTakeoutImport.Register(root, c)')
    })

    it('rejects a module path that would break out of the generated Go import', () => {
        const bad: CliPkg = { ...mail, module: 'tinycld.org/x"; evil()//' }
        expect(() => buildCliExtensionsSource([bad])).toThrow(/unsafe value/)
    })
})

describe('buildCliGoWork', () => {
    it('includes . and each cli package use, but no core line', () => {
        const work = buildCliGoWork([mail])
        expect(work).toContain('use (')
        expect(work).toContain('    .')
        expect(work).toContain('    ../../mail/cli')
        expect(work).not.toContain('core/server')
    })

    // Members require tinycld.org/cli v0.0.0; without a versioned workspace
    // replace the graph load hits the proxy and fails. Unversioned is rejected
    // ("replaced at all versions") because the module is itself a `use` member.
    it('replaces tinycld.org/cli (versioned) when members are present', () => {
        const work = buildCliGoWork([mail])
        expect(work).toContain('replace tinycld.org/cli v0.0.0 => .')
    })

    it('is a valid single-module workspace when no package declares cli', () => {
        const work = buildCliGoWork([])
        expect(work).toContain('use (')
        expect(work).toContain('    .')
        expect(work).not.toContain('replace')
    })
})

describe('buildSearchSlugsSource', () => {
    it('emits the slugs of packages declaring search, sorted', () => {
        const go = buildSearchSlugsSource(['mail', 'boards', 'drive'])
        expect(go).toContain('var searchSlugs = []string{"boards", "drive", "mail"}')
    })

    // The search set is NOT the cli set: boards and contacts contribute a search
    // source but ship no CLI commands, so deriving one list from the other
    // would make `cards:` parse as a literal word in the terminal.
    it('emits an empty slice when no package declares search', () => {
        const go = buildSearchSlugsSource([])
        expect(go).toContain('var searchSlugs = []string{}')
    })

    it('rejects a slug that would break out of the generated Go literal', () => {
        expect(() => buildSearchSlugsSource(['mail"; evil()//'])).toThrow(/unsafe value/)
    })
})

describe('buildMemberCliGoWork', () => {
    it('replaces tinycld.org/cli so a standalone member build resolves it', () => {
        const work = buildMemberCliGoWork('../../tinycld/cli')
        expect(work).toContain('use .')
        expect(work).toContain('replace tinycld.org/cli => ../../tinycld/cli')
    })
})
