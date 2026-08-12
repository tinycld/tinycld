import { useFileToken } from '@tinycld/core/file-viewer/use-authed-file-url'
import { pb } from '@tinycld/core/lib/pocketbase'
import { NodeViewWrapper, type ReactNodeViewProps } from '@tiptap/react'
import { resolveProtectedFileSrc } from './authed-image'

/**
 * Replaces Tiptap's default Image DOM so a protected PocketBase file renders:
 * the stored src is tokenless (see authed-image.ts), and a bare <img> would
 * 404 against the record's viewRule. A fresh per-user token is attached at
 * render time; data: URIs, external URLs and already-tokened srcs pass
 * through untouched. (text's ImageNodeView does the same job with resize and
 * wrap chrome on top — this is deliberately just the authed rendering.)
 */
export function AuthedImageView({ node }: ReactNodeViewProps<HTMLSpanElement>) {
    const src = (node.attrs.src as string | null) ?? ''
    const alt = (node.attrs.alt as string | null) ?? ''
    const title = (node.attrs.title as string | null) ?? ''
    const { data: token } = useFileToken()
    const displaySrc = resolveProtectedFileSrc(src, pb.baseURL, token)

    return (
        <NodeViewWrapper as="span" style={{ display: 'inline-block', lineHeight: 0 }}>
            <img
                src={displaySrc}
                alt={alt || undefined}
                title={title || undefined}
                draggable={false}
                style={{ maxWidth: '100%', height: 'auto' }}
            />
        </NodeViewWrapper>
    )
}
