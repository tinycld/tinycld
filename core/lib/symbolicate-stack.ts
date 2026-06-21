import { originalPositionFor, TraceMap } from '@jridgewell/trace-mapping'
import { parse as parseStack } from 'stacktrace-parser'

// Client-side stack symbolication for the web build.
//
// The browser does NOT apply sourcemaps to error.stack at runtime — that only
// happens in the DevTools UI (and in Sentry, server-side). So an uncaught error
// reaching our boundary carries MINIFIED frames (…/entry-abc.js:258:53709).
// This walks those frames, fetches the matching served `.map` for each chunk,
// and rewrites each frame to its original source:line (…/crash-test.tsx:30:4),
// producing a stack a human can actually read in the console.
//
// Sentry has no client utility for this (its rewriteFramesIntegration only
// adjusts frame paths for server-side symbolication); @jridgewell/trace-mapping
// + stacktrace-parser are the lightweight pieces that do it, and both are
// already in the dependency graph.
//
// Best-effort by design: a frame whose map is missing/unreadable, or that isn't
// a served chunk (a browser-internal frame), is left exactly as-is. Symbolication
// must never throw or mask the original error.

// Matches the chunk URLs the Go server serves (PoolAssets at /_expo/static/...).
const SERVED_CHUNK_RE = /\/_expo\/static\/.+\.js$/

// Per-URL cache so a stack with many frames in the same chunk fetches its map
// once. null = we tried and failed (don't retry within this page session).
const mapCache = new Map<string, TraceMap | null>()

async function loadMap(mapUrl: string): Promise<TraceMap | null> {
    const cached = mapCache.get(mapUrl)
    if (cached !== undefined) return cached
    try {
        const res = await fetch(mapUrl)
        if (!res.ok) {
            mapCache.set(mapUrl, null)
            return null
        }
        const map = new TraceMap(await res.json())
        mapCache.set(mapUrl, map)
        return map
    } catch {
        mapCache.set(mapUrl, null)
        return null
    }
}

// Turn a raw error stack into one whose served-chunk frames point at original
// source. Returns the rewritten multi-line string (header line preserved).
export async function symbolicateStack(stack: string): Promise<string> {
    const frames = parseStack(stack)
    if (frames.length === 0) return stack

    // The first line of error.stack is the "Error: message" header, not a frame.
    const header = stack.split('\n', 1)[0]

    const lines = await Promise.all(
        frames.map(async frame => {
            const { file, lineNumber, column, methodName } = frame
            const name = methodName && methodName !== '<unknown>' ? methodName : '<anonymous>'
            if (!file || lineNumber == null || !SERVED_CHUNK_RE.test(file)) {
                return `    at ${name} (${file ?? '<unknown>'}:${lineNumber ?? 0}:${column ?? 0})`
            }
            const map = await loadMap(`${file}.map`)
            if (!map) {
                return `    at ${name} (${file}:${lineNumber}:${column ?? 0})`
            }
            // stack columns are 1-based; trace-mapping is 0-based.
            const pos = originalPositionFor(map, {
                line: lineNumber,
                column: (column ?? 1) - 1,
            })
            if (!pos.source) {
                return `    at ${name} (${file}:${lineNumber}:${column ?? 0})`
            }
            const fn = pos.name || name
            return `    at ${fn} (${pos.source}:${pos.line}:${pos.column})`
        })
    )

    return [header, ...lines].join('\n')
}
