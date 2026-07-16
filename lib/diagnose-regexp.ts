// PRIMARY OTA-crash diagnostic. Wraps the global RegExp constructor so the exact
// pattern compiled right before a fatal is recorded — Hermes aborts the process
// on an unsupported/oversized regex via RCTFatal (SIGABRT) before any JS-level
// handler can read which pattern did it, and the native crash report only shows
// `regExpConstructor` with no pattern source. This shim captures the source
// string itself.
//
// Two outputs, both consumed by the install-fatal-rollback handler and the
// next-launch report-bad upload (see use-app-updates.reportRevertedBundle):
//   - globalThis.__TINYCLD_LAST_REGEXP: the most-recent { pattern, flags }
//     successfully or unsuccessfully handed to `new RegExp` (so even a pattern
//     that aborts Hermes mid-compile — where our own try/catch never returns —
//     is still on record as "the last thing attempted").
//   - globalThis.__TINYCLD_REGEXP_ERROR: set when construction THROWS a catchable
//     JS error (e.g. "Invalid RegExp: …"), carrying the pattern + the message.
//
// Side-effect import; must run before any module that compiles a regex. No-op on
// web (we only chase the Hermes/OTA crash) and guarded so a double-import is safe.

interface RegExpDiag {
    pattern: string
    flags: string
    at: string
}
interface RegExpErrorDiag extends RegExpDiag {
    message: string
}

declare global {
    var __TINYCLD_LAST_REGEXP: RegExpDiag | undefined
    var __TINYCLD_REGEXP_ERROR: RegExpErrorDiag | undefined
    var __TINYCLD_REGEXP_SHIMMED: boolean | undefined
}

// Only instrument where the crash lives: native Hermes. On web the platform
// RegExp is fine and wrapping it just adds overhead. We can't import
// react-native's Platform here without pulling a heavier graph in this very-early
// shim, so detect Hermes directly (HermesInternal is the native-only global).
const isHermes = typeof (globalThis as { HermesInternal?: unknown }).HermesInternal !== 'undefined'

if (isHermes && !globalThis.__TINYCLD_REGEXP_SHIMMED) {
    globalThis.__TINYCLD_REGEXP_SHIMMED = true

    const NativeRegExp = globalThis.RegExp

    // `source` can be a RegExp (RegExp(/re/)) or string; normalize to a loggable
    // string without itself compiling anything.
    const sourceToString = (source: unknown): string => {
        if (source instanceof NativeRegExp) return source.source
        return source === undefined ? '' : String(source)
    }

    const recordAttempt = (pattern: string, flags: string) => {
        // Record BEFORE the native call so a pattern that aborts the process
        // mid-compile (no return, no throw we can catch) is still the last entry.
        // No per-compile console log: this fires on every regex the app
        // compiles, which floods the dev console. The recorded
        // __TINYCLD_LAST_REGEXP (read by install-fatal-rollback + the
        // report-bad upload) is what actually diagnoses the crash.
        globalThis.__TINYCLD_LAST_REGEXP = {
            pattern,
            flags,
            at: new Date().toISOString(),
        }
    }

    // The wrapper must behave EXACTLY like RegExp for both `new RegExp(...)` and
    // `RegExp(...)` call forms, and preserve prototype/instanceof, or it would
    // change app behavior. We delegate to the native constructor via Reflect so
    // subclassing and the call-vs-construct distinction are preserved.
    function ShimRegExp(this: unknown, pattern?: unknown, flags?: unknown): RegExp {
        const patternStr = sourceToString(pattern)
        // RegExp(/re/) copies the source's flags when flags is undefined.
        const flagsStr =
            flags === undefined && pattern instanceof NativeRegExp
                ? pattern.flags
                : flags === undefined
                  ? ''
                  : String(flags)
        recordAttempt(patternStr, flagsStr)
        try {
            return Reflect.construct(
                NativeRegExp,
                flags === undefined ? [pattern] : [pattern, flags],
                ShimRegExp as unknown as new () => RegExp
            )
        } catch (err) {
            // A catchable construction error (Hermes throws "Invalid RegExp: …"
            // for some unsupported patterns rather than aborting). Stash the full
            // detail so the fatal handler + report-bad upload carry the exact
            // pattern and reason, then rethrow unchanged.
            globalThis.__TINYCLD_REGEXP_ERROR = {
                pattern: patternStr,
                flags: flagsStr,
                message: err instanceof Error ? err.message : String(err),
                at: new Date().toISOString(),
            }
            throw err
        }
    }

    // Preserve the prototype chain so `x instanceof RegExp`, RegExp.prototype
    // methods, and Symbol.species all keep working.
    ShimRegExp.prototype = NativeRegExp.prototype
    Object.setPrototypeOf(ShimRegExp, NativeRegExp)

    globalThis.RegExp = ShimRegExp as unknown as RegExpConstructor
}

export {}
