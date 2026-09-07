import { DocumentWebViewHost, type DocumentWebViewHostOptions } from './document-webview-host'
import {
    MARKDOWN_GET,
    MARKDOWN_RESULT,
    MARKDOWN_SET,
    type MarkdownResultPayload,
    type MarkdownSetPayload,
} from './webview/source/protocol'

export type MarkdownWebViewHostOptions = DocumentWebViewHostOptions

/**
 * Host side of the markdown channel — the save path for card descriptions.
 * The request/response mechanics and their failure modes are documented on
 * {@link DocumentWebViewHost}.
 */
export class MarkdownWebViewHost extends DocumentWebViewHost<string> {
    constructor(options: MarkdownWebViewHostOptions) {
        super(
            {
                namespace: 'markdown',
                types: { set: MARKDOWN_SET, get: MARKDOWN_GET, result: MARKDOWN_RESULT },
                initial: '',
                decodeResult: payload => {
                    const markdown = (payload as MarkdownResultPayload | undefined)?.markdown
                    return typeof markdown === 'string' ? markdown : null
                },
                encodeSet: (markdown): MarkdownSetPayload => ({ markdown }),
            },
            options
        )
    }
}
