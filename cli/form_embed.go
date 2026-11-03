package main

import "strings"

// Mirrors core/lib/web-forms/embed-code.ts; both read
// core/lib/web-forms/fixtures/embed-cases.json in their tests.
func publicFormURL(origin, token string) string {
	return origin + "/p/forms/" + token
}

var attrEscaper = strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;")

func embedCode(origin, token, title string) string {
	return `<iframe src="` + publicFormURL(origin, token) + `" data-tinycld-form title="` + attrEscaper.Replace(title) +
		`" style="width:100%;border:0" loading="lazy"></iframe>` + "\n" +
		`<script src="` + origin + `/forms-embed.js" async></script>`
}
