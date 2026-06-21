import { originalPositionFor, TraceMap } from '@jridgewell/trace-mapping'
import { expect, test } from '@playwright/test'

// Proves the web build ships working sourcemaps end-to-end:
//   1. /crash-test?boom=1 throws a render error from app/crash-test.tsx.
//   2. The root ErrorBoundary (app/_layout.tsx → AppErrorBoundary) catches it,
//      renders the fallback, reports to Sentry, AND symbolicates the stack
//      against the served `.map` files, console.logging the readable stack.
//   3. We assert the symbolicated console output names the ORIGINAL source
//      (crash-test.tsx) — i.e. the runtime client-side mapping worked.
//   4. As an independent check, we also pull a generated frame off the on-screen
//      stack, fetch that chunk's served `.map` ourselves, and resolve it with
//      @jridgewell/trace-mapping — confirming the served map is present + correct.
//
// Why this can't "just read the console" for the original location: the browser
// does NOT apply sourcemaps to error.stack at runtime — that translation only
// happens in the DevTools UI (and in Sentry, server-side). The readable stack
// here exists ONLY because AppErrorBoundary symbolicates it itself; and step 4
// re-derives the mapping independently. Either path fails loudly if the maps are
// missing or wrong, which is the point.

// A generated frame in the bundled output, e.g.
//   at f (http://localhost:7200/_expo/static/js/web/crash-test-ab12.js:1:378)
const FRAME_RE = /(\/_expo\/static\/js\/web\/[^\s):]+\.js):(\d+):(\d+)/g

test('web build serves sourcemaps that resolve a crash back to its source', async ({
    page,
    baseURL,
}) => {
    // AppErrorBoundary symbolicates the caught stack and logs it as
    // `[error-boundary] Error: …\n    at … (…/crash-test.tsx:line:col)`.
    const boundaryLogs: string[] = []
    page.on('console', msg => {
        const text = msg.text()
        if (text.includes('[error-boundary]')) boundaryLogs.push(text)
    })

    // Direct load of a standalone route is the legitimate use of goto (initial
    // navigation, not in-app navigation). The render throw is caught by the root
    // boundary.
    await page.goto('/crash-test?boom=1')

    // 1. The boundary caught the crash and rendered its fallback.
    await expect(page.getByTestId('error-boundary')).toBeVisible()

    // 2. The boundary symbolicated the stack and logged the ORIGINAL source.
    //    This is the runtime client-side sourcemap translation working.
    await expect
        .poll(() => boundaryLogs.join('\n'), {
            message: 'error boundary should log a symbolicated stack naming crash-test.tsx',
            timeout: 15_000,
        })
        .toContain('crash-test.tsx')

    // 3. Independent verification that the SERVED map is present + correct: pull
    //    a generated frame off the on-screen (un-symbolicated) stack, fetch its
    //    .map, and resolve it ourselves.
    const stack = (await page.getByTestId('error-boundary-stack').innerText()).trim()
    const frames = [...stack.matchAll(FRAME_RE)]
    expect(
        frames.length,
        `expected at least one /_expo/static frame in the stack:\n${stack}`
    ).toBeGreaterThan(0)

    const origin = baseURL ?? 'http://localhost:7200'
    const resolved: string[] = []
    for (const [, chunkPath, lineStr, colStr] of frames) {
        const mapUrl = `${origin}${chunkPath}.map`
        const res = await page.request.get(mapUrl)
        expect(res.ok(), `sourcemap not served at ${mapUrl} (status ${res.status()})`).toBe(true)

        const map = new TraceMap(await res.json())
        const pos = originalPositionFor(map, {
            line: Number(lineStr),
            // stack columns are 1-based; trace-mapping is 0-based.
            column: Number(colStr) - 1,
        })
        if (pos.source) resolved.push(pos.source)
    }

    expect(
        resolved.some(s => s.endsWith('crash-test.tsx')),
        `no served-map frame resolved to crash-test.tsx; resolved sources were:\n${resolved.join('\n')}`
    ).toBe(true)
})
