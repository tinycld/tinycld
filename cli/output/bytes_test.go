package output

import "testing"

// The expectations pin parity with core/server/quota.FormatBytes — if either
// side changes its rendering, this table is the tripwire.
func TestFormatBytesMatchesQuotaPackage(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{5 * 1024 * 1024 * 1024, "5.0 GB"},
		{3 * 1024 * 1024 * 1024 * 1024, "3.0 TB"},
		// Beyond TB quota.FormatBytes caps the LABEL at TB while the divisor
		// keeps scaling, so 2 PB renders as "2.0 TB". Quirk preserved: parity
		// with the app matters more than petabyte cosmetics.
		{2 * 1024 * 1024 * 1024 * 1024 * 1024, "2.0 TB"},
	}
	for _, c := range cases {
		if got := FormatBytes(c.in); got != c.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
