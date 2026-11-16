package pdfrender

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// ContentDisposition names a download: stem, then "." and ext (pdf, csv,
// xlsx). A name can come from a person (a customer, a report title), so
// anything that could break the header or climb a directory is replaced. A
// name with non-ASCII letters also carries its real spelling in an RFC 6266
// filename* parameter; the quoted filename stays as the ASCII fallback for old
// clients.
func ContentDisposition(stem, ext string) string {
	fallback := strings.Trim(unsafeFileChars.ReplaceAllString(stem, "-"), "-.")
	fallback = strings.ToLower(fallback)
	if fallback == "" {
		fallback = "document"
	}
	header := `attachment; filename="` + fallback + "." + ext + `"`
	if isASCII(stem) {
		return header
	}
	return header + `; filename*=UTF-8''` + percentEncodeFileName(realFileName(stem)+"."+ext)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// realFileName keeps the person's spelling but drops what could climb a
// directory or break a header: path separators, quotes, control characters and
// the bidi controls (including the Arabic letter mark) and byte-order mark
// that could spoof the displayed name. Zero-width joiners stay: Persian, Indic
// names and emoji need them.
func realFileName(stem string) string {
	clean := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == '"' || r == utf8.RuneError ||
			unicode.IsControl(r) || isSpoofingFormat(r) {
			return -1
		}
		return r
	}, stem)
	clean = strings.Trim(clean, " .")
	if clean == "" {
		return "document"
	}
	return clean
}

func isSpoofingFormat(r rune) bool {
	return r == 0x061C || r == 0x200E || r == 0x200F || r == 0xFEFF ||
		(r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}

// percentEncodeFileName encodes every byte outside RFC 5987's attr-char set.
func percentEncodeFileName(name string) string {
	const attrPunct = "!#$&+-.^_`|~"
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		alnum := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
		if alnum || strings.IndexByte(attrPunct, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}
