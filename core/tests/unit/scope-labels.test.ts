import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { SCOPE_LABELS } from '../../components/oauth/ScopeList'

// The consent screen renders SCOPE_LABELS[scope] ?? scope, so a scope with no
// label asks a person to approve the raw string — "boards:write" instead of
// "Create and modify your boards and cards". That is exactly what shipped: the
// cards scopes went out unlabeled and nothing caught it, because the label map
// and the scope catalog live in different languages.
//
// This reads the Go catalog as the source of truth rather than restating it,
// so adding a scope in oauth.go without a label turns this red.
const OAUTH_GO = join(__dirname, '../../server/oauth/oauth.go')

// Scope constants are declared as `ScopeMailRead = "mail:read"`. Reading the
// literals (rather than the AllScopes identifier list) keeps this immune to
// how that slice is formatted.
function scopesFromGo(): string[] {
    const source = readFileSync(OAUTH_GO, 'utf8')
    const start = source.indexOf('var AllScopes = []string{')
    // Skip the declaration line itself, whose `AllScopes` identifier would
    // otherwise match the constant pattern below.
    const body = source.slice(source.indexOf('\n', start), source.indexOf('}', start))
    const names = [...new Set([...body.matchAll(/\bScope[A-Za-z]+\b/g)].map(m => m[0]))]
    return names.map(name => {
        const declared = source.match(new RegExp(`${name}\\s*=\\s*"([^"]+)"`))
        if (!declared) throw new Error(`no string literal found for ${name} in oauth.go`)
        return declared[1]
    })
}

describe('OAuth consent scope labels', () => {
    it('finds the Go scope catalog', () => {
        const scopes = scopesFromGo()
        // A parse failure would otherwise make the next test vacuously pass.
        expect(scopes.length).toBeGreaterThanOrEqual(11)
        expect(scopes).toContain('mail:read')
    })

    it('labels every scope the server can grant', () => {
        const unlabeled = scopesFromGo().filter(scope => !SCOPE_LABELS[scope])
        expect(unlabeled).toEqual([])
    })

    it('labels nothing the server cannot grant', () => {
        // A stale label is a smaller problem than a missing one, but it still
        // means the map and the catalog disagree about what exists.
        const known = new Set(scopesFromGo())
        expect(Object.keys(SCOPE_LABELS).filter(scope => !known.has(scope))).toEqual([])
    })

    it('writes labels as plain language, not the scope string', () => {
        for (const [scope, label] of Object.entries(SCOPE_LABELS)) {
            expect(label, `${scope} should read as a sentence`).not.toContain(':')
        }
    })
})
