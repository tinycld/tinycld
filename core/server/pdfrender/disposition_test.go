package pdfrender

import "testing"

// Non-ASCII stems are written as byte escapes, so the bidi and zero-width
// controls stay visible in the source.
const (
	eAcute = "\xc3\xa9"     // é
	rlo    = "\xe2\x80\xae" // U+202E right-to-left override
	bom    = "\xef\xbb\xbf" // U+FEFF zero-width no-break space
	alm    = "\xd8\x9c"     // U+061C Arabic letter mark
	zwnj   = "\xe2\x80\x8c" // U+200C zero-width non-joiner
	nihon  = "\xe6\x97\xa5\xe6\x9c\xac"
	// Persian "می‌خواهم": mim, yeh, ZWNJ, kheh, waw, alef, heh, mim.
	persian = "\xd9\x85\xdb\x8c" + zwnj + "\xd8\xae\xd9\x88\xd8\xa7\xd9\x87\xd9\x85"
)

func TestContentDisposition_SanitizesTheName(t *testing.T) {
	const attach = `attachment; filename=`
	for _, tc := range []struct{ stem, ext, want string }{
		{"sample-document", "pdf", attach + `"sample-document.pdf"`},
		{`Document 1042/"x"`, "pdf", attach + `"document-1042-x.pdf"`},
		{"../..", "pdf", attach + `"document.pdf"`},
		{"", "pdf", attach + `"document.pdf"`},
		{"Caf" + eAcute + " Ltd", "pdf",
			attach + `"caf-ltd.pdf"; filename*=UTF-8''Caf%C3%A9%20Ltd.pdf`},
		{nihon, "pdf", attach + `"document.pdf"; filename*=UTF-8''%E6%97%A5%E6%9C%AC.pdf`},
		{eAcute + "/\"..\\x\n", "pdf", attach + `"x.pdf"; filename*=UTF-8''%C3%A9..x.pdf`},
		{"Caf" + eAcute + rlo + "txt." + bom + "exe", "pdf",
			attach + `"caf-txt.-exe.pdf"; filename*=UTF-8''Caf%C3%A9txt.exe.pdf`},
		{"Caf" + eAcute + alm + "txt", "pdf",
			attach + `"caf-txt.pdf"; filename*=UTF-8''Caf%C3%A9txt.pdf`},
		{persian, "pdf", attach + `"document.pdf"; filename*=UTF-8''%D9%85%DB%8C%E2%80%8C` +
			`%D8%AE%D9%88%D8%A7%D9%87%D9%85.pdf`},

		// csv and xlsx end the name exactly as pdf does.
		{"Zoo Sales", "csv", attach + `"zoo-sales.csv"`},
		{"Zoo Sales", "xlsx", attach + `"zoo-sales.xlsx"`},
		{"", "csv", attach + `"document.csv"`},
		{"../..", "xlsx", attach + `"document.xlsx"`},
		{"Caf" + eAcute + " Ltd", "csv",
			attach + `"caf-ltd.csv"; filename*=UTF-8''Caf%C3%A9%20Ltd.csv`},
		{nihon, "xlsx", attach + `"document.xlsx"; filename*=UTF-8''%E6%97%A5%E6%9C%AC.xlsx`},
		{eAcute + "/\"..\\x\n", "csv", attach + `"x.csv"; filename*=UTF-8''%C3%A9..x.csv`},
		{"Caf" + eAcute + rlo + "txt." + bom + "exe", "xlsx",
			attach + `"caf-txt.-exe.xlsx"; filename*=UTF-8''Caf%C3%A9txt.exe.xlsx`},
	} {
		if got := ContentDisposition(tc.stem, tc.ext); got != tc.want {
			t.Errorf("ContentDisposition(%q, %q) = %q, want %q", tc.stem, tc.ext, got, tc.want)
		}
	}
}
