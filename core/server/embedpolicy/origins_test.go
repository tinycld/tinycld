package embedpolicy

import (
	"strings"
	"testing"
)

func TestParseOrigins(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"", "", true},
		{"https://a.example  https://a.example\nhttp://b.example:8080/", "https://a.example http://b.example:8080", true},
		{"a.example", "", false},
		{"ftp://a.example", "", false},
		{"https://a.example/path", "", false},
		{"https://*.example", "", false},
		{"https://a.example;script-src", "", false},
		{"https://u:p@a.example", "", false},
	}
	for _, c := range cases {
		got, err := ParseOrigins(c.in)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("ParseOrigins(%q) = %q, %v; want %q ok=%v", c.in, got, err, c.want, c.ok)
		}
	}
	many := make([]string, 0, MaxOrigins+1)
	for i := 0; i <= MaxOrigins; i++ {
		many = append(many, "https://h"+string(rune('a'+i))+".example")
	}
	if _, err := ParseOrigins(strings.Join(many, " ")); err == nil {
		t.Error("want an error over MaxOrigins")
	}
}
