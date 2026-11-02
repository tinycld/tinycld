// Helper for pages that embed a tinycld web form. Sizes each
// iframe[data-tinycld-form] to its content and re-dispatches the form's
// "submitted" message as a DOM event on the iframe, so the host page can
// redirect or record a conversion:
//   iframe.addEventListener('tinycld:form-submitted', e => { ... e.detail.form })
;(() => {
    if (window.__tinycldFormsEmbed) return
    window.__tinycldFormsEmbed = true

    const frameFor = source => {
        for (const frame of document.querySelectorAll('iframe[data-tinycld-form]')) {
            if (frame.contentWindow === source) return frame
        }
        return null
    }

    window.addEventListener('message', event => {
        const frame = frameFor(event.source)
        if (!frame) return
        // Only the iframe's own origin may resize it or claim a submission.
        if (new URL(frame.src).origin !== event.origin) return
        const data = event.data
        if (!data || typeof data.type !== 'string') return
        if (data.type === 'tinycld:form-resize' && typeof data.height === 'number') {
            frame.style.height = `${Math.ceil(data.height)}px`
        }
        if (data.type === 'tinycld:form-submitted') {
            frame.dispatchEvent(
                new CustomEvent('tinycld:form-submitted', {
                    bubbles: true,
                    detail: { form: data.form },
                })
            )
        }
    })
})()
