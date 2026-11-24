import * as fs from 'node:fs'
import * as path from 'node:path'
import type { PackageManifest } from './load-manifest'

// A terms version is an opaque label the package author changes when the text
// changes (a date or a number). It is compared for equality only.
export const TERMS_VERSION_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{0,49}$/
// Keep in step with the `body` max of pkg_terms
// (core/server/pb_migrations/2090000017_create_pkg_terms.js).
export const TERMS_MAX_BYTES = 200_000

export interface ResolvedTerms {
    version: string
    body: string
}

// Validate a manifest's `terms` and read its markdown file. Returns undefined
// for a package without terms. Throws with the package slug on a bad
// declaration, so a generate run stops before it writes a registry row that
// would block the package with no text to show.
export function resolveTerms(
    packageDir: string,
    slug: string,
    terms: PackageManifest['terms']
): ResolvedTerms | undefined {
    if (terms === undefined) return undefined
    const fail = (why: string): never => {
        throw new Error(`[generate] ${slug}: manifest.terms ${why}`)
    }
    if (typeof terms.version !== 'string' || !TERMS_VERSION_PATTERN.test(terms.version)) {
        fail(
            `version ${JSON.stringify(terms.version)} is not valid: use 1 to 50 letters, digits, '.', '_' or '-', starting with a letter or digit`
        )
    }
    if (typeof terms.file !== 'string' || !terms.file.endsWith('.md')) {
        fail(
            `file ${JSON.stringify(terms.file)} must be a .md file (lowercase extension) in the package`
        )
    }
    const root = path.resolve(packageDir)
    const lexical = path.resolve(root, terms.file)
    if (!lexical.startsWith(root + path.sep)) {
        fail(`file ${JSON.stringify(terms.file)} is outside the package`)
    }
    if (!fs.existsSync(lexical) || !fs.statSync(lexical).isFile()) {
        fail(`file ${JSON.stringify(terms.file)} does not exist`)
    }
    // A symlink inside the package can point out of it; compare real paths.
    const realRoot = fs.realpathSync(root)
    const file = fs.realpathSync(lexical)
    if (!file.startsWith(realRoot + path.sep)) {
        fail(`file ${JSON.stringify(terms.file)} is outside the package`)
    }
    const body = fs.readFileSync(file, 'utf8').trim()
    if (body === '') fail(`file ${JSON.stringify(terms.file)} is empty`)
    if (Buffer.byteLength(body, 'utf8') > TERMS_MAX_BYTES) {
        fail(`file ${JSON.stringify(terms.file)} is larger than ${TERMS_MAX_BYTES} bytes`)
    }
    return { version: terms.version, body }
}
