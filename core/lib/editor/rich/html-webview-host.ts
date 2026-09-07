import { DocumentWebViewHost, type DocumentWebViewHostOptions } from './document-webview-host'
import {
    HTML_GET,
    HTML_RESULT,
    HTML_SET,
    type HtmlResultPayload,
    type HtmlSetPayload,
} from './webview/source/protocol'

/**
 * What the page reports on an html `get`. The text is what Tiptap's getText()
 * yields for the same document; it rides along because the host cannot derive
 * it from the HTML, and every caller that wants one wants the other.
 */
export type HtmlDocument = HtmlResultPayload

export type HtmlWebViewHostOptions = DocumentWebViewHostOptions

/**
 * Host side of the html channel: the `getHTML` / `getText` / `setContent`
 * surface of the shared editor handle on native. Mail's compose window is the
 * main caller — closing it reads the body to decide whether to save a draft,
 * and sending reads both parts. The request/response mechanics and their
 * failure modes are documented on {@link DocumentWebViewHost}.
 */
export class HtmlWebViewHost extends DocumentWebViewHost<HtmlDocument> {
    constructor(options: HtmlWebViewHostOptions) {
        super(
            {
                namespace: 'html',
                types: { set: HTML_SET, get: HTML_GET, result: HTML_RESULT },
                initial: { html: '', text: '' },
                decodeResult: payload => {
                    const result = payload as Partial<HtmlResultPayload> | undefined
                    if (typeof result?.html !== 'string' || typeof result.text !== 'string') {
                        return null
                    }
                    return { html: result.html, text: result.text }
                },
                encodeSet: ({ html }): HtmlSetPayload => ({ html }),
            },
            options
        )
    }

    /**
     * Replace the document with an HTML string. The plain-text fallback is
     * cleared rather than guessed: the host has no parser, and the next
     * successful round-trip restores it.
     */
    setHtml(html: string): void {
        this.set({ html, text: '' })
    }
}
