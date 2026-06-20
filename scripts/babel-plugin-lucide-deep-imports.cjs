// Babel plugin: rewrite barrel imports of lucide-react-native into per-icon
// deep imports so Metro (which doesn't tree-shake) only bundles the icons a file
// actually uses.
//
//   import { Folder, CircleHelp, type LucideIcon } from 'lucide-react-native'
//     ->
//   import type { LucideIcon } from 'lucide-react-native'
//   import Folder from 'lucide-react-native/icons/folder'
//   import CircleHelp from 'lucide-react-native/icons/circle-question-mark'
//
// The `lucide-react-native/icons/<kebab>` specifier is remapped to the real
// per-icon module by metro.config.cjs (lucide's exports map blocks the bare deep
// path). Without this, a handful of named icons drags the whole ~1700-icon
// barrel (~3.3 MB) into every chunk that imports them.
//
// The identifier -> icon-file map (including every lucide alias, e.g.
// HelpCircle/CircleHelp -> circle-question-mark) is parsed once from lucide's
// own barrel re-export statements, so it stays correct across lucide versions
// without hand-maintained tables.

const fs = require('node:fs')
const path = require('node:path')

const SOURCE = 'lucide-react-native'
const ICON_SPECIFIER_BASE = `${SOURCE}/icons/`

// Build { ExportedName: 'kebab-file' } from lucide's barrel. Resolve the package
// via its allowed main entry, then walk up to the package root (the exports map
// blocks resolving ./package.json or ./dist/* directly).
function buildIconMap() {
    let dir = path.dirname(require.resolve(SOURCE))
    while (path.basename(dir) !== SOURCE && dir !== path.dirname(dir)) {
        dir = path.dirname(dir)
    }
    const barrelPath = path.join(dir, 'dist', 'esm', `${SOURCE}.mjs`)
    const barrel = fs.readFileSync(barrelPath, 'utf8')
    // Matches: export { default as A, default as B } from './icons/<file>.mjs'
    const reExport = /export\s*\{([^}]*)\}\s*from\s*'\.\/icons\/([a-z0-9-]+)\.mjs'/g
    const map = new Map()
    let m
    while ((m = reExport.exec(barrel))) {
        const file = m[2]
        for (const part of m[1].split(',')) {
            const named = part.trim().match(/default as ([A-Za-z0-9_]+)/)
            if (named) map.set(named[1], file)
        }
    }
    if (map.size === 0) {
        throw new Error(
            `[lucide-deep-imports] parsed 0 icons from ${barrelPath} — lucide layout changed?`
        )
    }
    return map
}

let ICON_MAP = null

module.exports = function lucideDeepImports({ types: t }) {
    if (!ICON_MAP) ICON_MAP = buildIconMap()

    return {
        name: 'lucide-deep-imports',
        visitor: {
            ImportDeclaration(p) {
                if (p.node.source.value !== SOURCE) return
                // A whole-module type import (`import type { X } from 'lucide…'`)
                // is erased at compile — leave it on the barrel untouched.
                if (p.node.importKind === 'type') return

                const typeSpecifiers = [] // keep on the barrel (types, erased)
                const iconImports = [] // new per-icon default imports
                const keepOnBarrel = [] // anything we can't safely rewrite

                for (const spec of p.node.specifiers) {
                    // Only named imports are rewritable; a default or namespace
                    // import of lucide isn't an icon, so leave the decl alone.
                    if (!t.isImportSpecifier(spec)) return
                    const imported = t.isIdentifier(spec.imported)
                        ? spec.imported.name
                        : spec.imported.value
                    if (spec.importKind === 'type') {
                        // Moving this into an `import type { … }` declaration, so
                        // drop the per-specifier `type` keyword (else it emits the
                        // invalid `import type { type LucideIcon }`).
                        spec.importKind = null
                        typeSpecifiers.push(spec)
                        continue
                    }
                    const file = ICON_MAP.get(imported)
                    if (!file) {
                        // Unknown identifier (not an icon, or a non-icon value
                        // export) — keep it on the barrel so behavior is unchanged.
                        keepOnBarrel.push(spec)
                        continue
                    }
                    iconImports.push(
                        t.importDeclaration(
                            [t.importDefaultSpecifier(t.identifier(spec.local.name))],
                            t.stringLiteral(`${ICON_SPECIFIER_BASE}${file}`)
                        )
                    )
                }

                if (iconImports.length === 0) return // nothing to split

                const replacements = [...iconImports]
                if (typeSpecifiers.length > 0) {
                    const typeDecl = t.importDeclaration(typeSpecifiers, t.stringLiteral(SOURCE))
                    typeDecl.importKind = 'type'
                    replacements.push(typeDecl)
                }
                if (keepOnBarrel.length > 0) {
                    replacements.push(
                        t.importDeclaration(keepOnBarrel, t.stringLiteral(SOURCE))
                    )
                }
                p.replaceWithMultiple(replacements)
            },
        },
    }
}
