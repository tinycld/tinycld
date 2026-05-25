import { useEffect, useMemo, useRef, useState } from 'react'
import { View } from 'react-native'

// envelope wraps the server-rendered fragment in a minimal HTML
// document so the iframe is fully self-contained. The {css} block is
// the surface-specific styling (preview vs print); the {html} body is
// the server-emitted fragment unchanged.
//
// The inline resize-observer at the bottom posts the body's
// scrollHeight back to the parent so we can size the iframe to its
// content. This avoids the alternative of either fixed-height
// scrollable iframes (cramped) or `height: 100vh` (no internal scroll).
const RESIZE_SCRIPT = `<script>
(function(){
    function post() {
        var height = document.documentElement.scrollHeight || document.body.scrollHeight || 0;
        try { parent.postMessage({ type: 'tinycld-html-surface-size', height: height }, '*'); } catch (e) {}
    }
    if (typeof ResizeObserver === 'function') {
        var ro = new ResizeObserver(post);
        ro.observe(document.documentElement);
        ro.observe(document.body);
    }
    window.addEventListener('load', post);
    setTimeout(post, 0);
})();
</script>`

// CALC_ANCHOR_SCRIPT lets a click inside the rendered grid post a cell
// anchor {sheet,row,col,quoted} to the parent. It derives coordinates
// purely from the DOM the calc renderer already emits: the row number
// from the row's leading `.tinycld-calc-row-h`, the column letter by
// matching the clicked cell's index against the `.tinycld-calc-col-h`
// header cells. No per-cell data-* attributes required.
const CALC_ANCHOR_SCRIPT = `<script>
(function(){
    function colLetterFor(td){
        var tr = td.parentElement;
        if(!tr) return '';
        var idx = -1, i, cells = tr.children;
        for(i=0;i<cells.length;i++){ if(cells[i]===td){ idx=i; break; } }
        if(idx<0) return '';
        var thead = document.querySelector('.tinycld-calc-grid thead tr');
        if(!thead) return '';
        var head = thead.children[idx];
        return head ? (head.textContent||'').trim() : '';
    }
    function rowNumFor(td){
        var tr = td.parentElement;
        if(!tr) return '';
        var rh = tr.querySelector('.tinycld-calc-row-h');
        return rh ? (rh.textContent||'').trim() : '';
    }
    document.addEventListener('click', function(ev){
        var td = ev.target && ev.target.closest ? ev.target.closest('td.tinycld-calc-cell') : null;
        if(!td) return;
        var col = colLetterFor(td);
        var row = rowNumFor(td);
        if(!col || !row) return;
        document.querySelectorAll('.tinycld-anchor-active').forEach(function(n){ n.classList.remove('tinycld-anchor-active'); });
        td.classList.add('tinycld-anchor-active');
        try { parent.postMessage({ type: 'tinycld-html-surface-anchor', kind: 'calc_cell', col: col, row: row, quoted: (td.textContent||'').trim().slice(0,280) }, '*'); } catch(e){}
    });
})();
</script>`

// TEXT_ANCHOR_SCRIPT lets a text selection inside the rendered document
// post a character-offset range {start,end,quoted} to the parent. Offsets
// are computed over the document's textContent so they survive across
// re-renders of the same content; quoted is a snapshot for re-anchoring
// when offsets drift.
const TEXT_ANCHOR_SCRIPT = `<script>
(function(){
    function offsetOf(root, node, nodeOffset){
        var walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, null);
        var total = 0, n;
        while((n = walker.nextNode())){
            if(n === node){ return total + nodeOffset; }
            total += n.nodeValue.length;
        }
        return total;
    }
    document.addEventListener('mouseup', function(){
        var sel = window.getSelection();
        if(!sel || sel.isCollapsed || sel.rangeCount===0) return;
        var range = sel.getRangeAt(0);
        var root = document.body;
        var start = offsetOf(root, range.startContainer, range.startOffset);
        var end = offsetOf(root, range.endContainer, range.endOffset);
        if(end<=start) return;
        var quoted = sel.toString().trim().slice(0,280);
        try { parent.postMessage({ type: 'tinycld-html-surface-anchor', kind: 'text_range', start: start, end: end, quoted: quoted }, '*'); } catch(e){}
    });
})();
</script>`

const ANCHOR_CSS = `.tinycld-anchor-active{ outline: 2px solid #f59e0b; outline-offset: -2px; }`

// parseAnchorMessage validates an anchor payload posted out of the
// sandboxed iframe before it reaches consumer code. The iframe runs
// allow-scripts over server-sanitized HTML, but it's still untrusted
// input crossing a frame boundary, so we check the discriminated-union
// shape rather than blindly casting. Returns null for anything
// malformed (the anchor is silently ignored).
function parseAnchorMessage(data: unknown): CommentAnchor | null {
    if (!data || typeof data !== 'object') return null
    const d = data as Record<string, unknown>
    if (d.kind === 'calc_cell') {
        if (typeof d.col !== 'string' || d.col === '') return null
        if (typeof d.row !== 'string' || d.row === '') return null
        return { kind: 'calc_cell', col: d.col, row: d.row, quoted: asQuoted(d.quoted) }
    }
    if (d.kind === 'text_range') {
        if (typeof d.start !== 'number' || typeof d.end !== 'number') return null
        if (!(d.end > d.start)) return null
        return { kind: 'text_range', start: d.start, end: d.end, quoted: asQuoted(d.quoted) }
    }
    return null
}

function asQuoted(v: unknown): string {
    return typeof v === 'string' ? v : ''
}

export type CommentAnchorMode = 'calc_cell' | 'text_range'

// CommentAnchor is the location a comment is attached to, posted out of
// the iframe when the visitor clicks a cell / selects text.
export type CommentAnchor =
    | { kind: 'calc_cell'; col: string; row: string; quoted: string }
    | { kind: 'text_range'; start: number; end: number; quoted: string }

// Exported for unit tests. Composes the envelope the iframe loads. When
// commentMode is set, the anchor interaction script + highlight CSS are
// appended.
export function buildHtmlSurfaceSrcDoc(
    html: string,
    css: string,
    commentMode?: CommentAnchorMode
): string {
    const anchorScript =
        commentMode === 'calc_cell'
            ? CALC_ANCHOR_SCRIPT
            : commentMode === 'text_range'
              ? TEXT_ANCHOR_SCRIPT
              : ''
    const anchorCss = commentMode ? ANCHOR_CSS : ''
    return `<!doctype html><html><head><meta charset="utf-8"><style>${css}${anchorCss}</style></head><body>${html}${RESIZE_SCRIPT}${anchorScript}</body></html>`
}

export interface HtmlSurfaceProps {
    html: string
    css: string
    ariaLabel?: string
    /** Enables click/selection anchoring for comments. */
    commentMode?: CommentAnchorMode
    /** Called when the visitor anchors a comment (click cell / select text). */
    onAnchor?: (anchor: CommentAnchor) => void
}

// HtmlSurface renders a server-emitted HTML fragment inside a
// sandboxed iframe (web) or a WebView (native). The iframe is
// `sandbox="allow-scripts"` — scripts execute in an opaque origin
// so the embedded resize-observer can post height updates, but the
// document has no access to the parent or its cookies. We
// deliberately do NOT combine `allow-same-origin` with
// `allow-scripts`: per the HTML spec, that combination lets the
// embedded script remove the sandbox attribute from
// `parent.document`, neutering the isolation.
//
// Image URLs in the fragment carry their auth tokens as query-string
// parameters (PocketBase's file-token scheme) which work cross-origin
// without cookies, so the opaque origin doesn't break image loading.
//
// Height tracking: the resize-observer script posts the body's
// scroll height back to the parent. We initialize the iframe at a
// small minimum so it's visible before the first post-message lands.
export function HtmlSurface({ html, css, ariaLabel, commentMode, onAnchor }: HtmlSurfaceProps) {
    const srcDoc = useMemo(
        () => buildHtmlSurfaceSrcDoc(html, css, commentMode),
        [html, css, commentMode]
    )
    const [height, setHeight] = useState(120)
    const iframeRef = useRef<HTMLIFrameElement | null>(null)
    const onAnchorRef = useRef(onAnchor)
    onAnchorRef.current = onAnchor

    useEffect(() => {
        const handler = (event: MessageEvent) => {
            // Sandbox-with-scripts (no allow-same-origin) means
            // event.source is an opaque-origin Window. We can still
            // identity-compare it against iframeRef.current.contentWindow
            // because both pointers refer to the same opaque
            // browsing-context object.
            if (event.source !== iframeRef.current?.contentWindow) return
            const data = event.data as
                | { type?: string; height?: number }
                | (CommentAnchor & { type?: string })
                | null
            if (!data || typeof data !== 'object') return
            if (data.type === 'tinycld-html-surface-size') {
                const h = (data as { height?: number }).height
                if (typeof h === 'number' && h > 0) setHeight(h)
                return
            }
            if (data.type === 'tinycld-html-surface-anchor') {
                const anchor = parseAnchorMessage(data)
                if (anchor) onAnchorRef.current?.(anchor)
            }
        }
        window.addEventListener('message', handler)
        return () => window.removeEventListener('message', handler)
    }, [])

    return (
        <View className="flex-1 bg-background">
            <iframe
                ref={iframeRef}
                title={ariaLabel ?? 'Rendered content'}
                srcDoc={srcDoc}
                sandbox="allow-scripts"
                aria-label={ariaLabel}
                style={{
                    border: 'none',
                    width: '100%',
                    height: `${height}px`,
                    display: 'block',
                    backgroundColor: 'transparent',
                }}
            />
        </View>
    )
}
