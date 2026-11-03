export function publicFormURL(origin: string, token: string): string {
    return `${origin}/p/forms/${token}`
}

function escapeAttr(s: string): string {
    return s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;')
}

/** The code an admin pastes into a website: the form's iframe and the helper that sizes it. */
export function embedCode(o: { origin: string; token: string; title: string }): string {
    const src = publicFormURL(o.origin, o.token)
    return (
        `<iframe src="${src}" data-tinycld-form title="${escapeAttr(o.title)}" style="width:100%;border:0" loading="lazy"></iframe>\n` +
        `<script src="${o.origin}/forms-embed.js" async></script>`
    )
}
