package filesystem

import "unicode/utf8"

// truncateOriginalName keeps at most the first 255 bytes of name. Upstream cut
// by bytes; the cut here moves back to a rune start, because the blob writer
// refuses metadata that is not valid UTF-8 and the upload then fails.
func truncateOriginalName(name string) string {
	if len(name) <= 255 {
		return name
	}
	cut := 255
	for cut > 0 && !utf8.RuneStart(name[cut]) {
		cut--
	}
	return name[:cut]
}
