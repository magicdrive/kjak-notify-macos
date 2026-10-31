package plugin

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// FuzzArgs: whatever the message, every value given to terminal-notifier
// starts with the backslash it removes (never "-": never an option), the
// title and the level are one line without control characters, and the body
// is valid UTF-8 with no control character but line breaks and tabs.
func FuzzArgs(f *testing.F) {
	f.Add("Build done", "all passed", "success", "https://ci.example/7")
	f.Add("-execute", "-message", "-help", "")
	f.Add("(a,b)", "{a=b;}", "<0001>", "\"q\"")
	f.Add("\x00\x1b[31m\r\n", "\xff ", "", "\n")
	f.Fuzz(func(t *testing.T, title, body, level, url string) {
		ti, sub, b := compose(Message{Title: title, Body: body, Level: level, URL: url})
		a := args(ti, sub)
		if a[0] != "-title" || len(a)%2 != 0 {
			t.Fatalf("%q", a)
		}
		for i := 1; i < len(a); i += 2 {
			if !strings.HasPrefix(a[i], `\`) {
				t.Fatalf("value %q", a[i])
			}
		}
		for _, s := range []string{ti, sub} {
			for _, r := range s {
				if unicode.IsControl(r) {
					t.Fatalf("control %U in %q", r, s)
				}
			}
		}
		if ti == "" || !utf8.ValidString(b) {
			t.Fatalf("%q %q", ti, b)
		}
		for _, r := range b {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				t.Fatalf("control %U in the body", r)
			}
		}
	})
}
