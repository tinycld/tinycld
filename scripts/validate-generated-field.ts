// Guards manifest-derived strings before they're interpolated UNESCAPED into
// generated TS/Go source (import specifiers, Go import paths, Go identifiers,
// route re-export paths). A stray quote or backslash in one of these would
// inject code into the generated file. The in-app installer regenerates on the
// SERVER from installed manifests, so this is a real (bounded) injection
// surface — validate against a strict allowlist and fail generation loudly
// rather than emit broken/injected code.

// Import specifiers, package names, Go module paths, Go identifiers, component
// subpaths, schema type names. Allows word chars, '.', '/', '-', '@' — enough
// for '@tinycld/mail', 'tinycld.org/packages/mail', 'system-settings/provider',
// 'ContactsSchema'. Rejects quotes, backslashes (also for Windows), whitespace,
// and any other injection vector.
const SAFE_IMPORT_FIELD = /^[\w./@-]+$/

// Route file paths additionally carry Expo Router bracket segments ('[id]',
// 'share/[token]'), so the route-path allowlist permits '[' and ']' on top of
// the import-field set.
const SAFE_ROUTE_PATH = /^[\w./@[\]-]+$/

// Assert a manifest-derived value is safe to interpolate into a generated
// import/identifier. Throws a clear, field-named error when it isn't.
export function assertSafeImportField(field: string, value: string): string {
    if (!SAFE_IMPORT_FIELD.test(value)) {
        throw new Error(
            `Refusing to generate: manifest field "${field}" has an unsafe value ${JSON.stringify(
                value
            )}. Only [A-Za-z0-9_./@-] are allowed (no quotes, backslashes, or whitespace) — it is interpolated unescaped into generated code.`
        )
    }
    return value
}

// Assert a route file path (relative, no extension) is safe to interpolate into
// a generated re-export specifier. Permits Expo Router bracket segments.
export function assertSafeRoutePath(field: string, value: string): string {
    if (!SAFE_ROUTE_PATH.test(value)) {
        throw new Error(
            `Refusing to generate: route path "${field}" has an unsafe value ${JSON.stringify(
                value
            )}. Only [A-Za-z0-9_./@[\\]-] are allowed (no quotes, backslashes, or whitespace) — it is interpolated unescaped into generated code.`
        )
    }
    return value
}
