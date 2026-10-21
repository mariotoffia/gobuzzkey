package shared

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// Printable makes text that came from outside (a relay response, a channel
// name, a file) safe to show on a terminal: C0 and C1 control characters, DEL
// and invalid UTF-8 are replaced by escapes, so a remote party cannot move the
// cursor, rewrite earlier lines or reach the clipboard through escape
// sequences. The result is cut to at most limit runes, marked with "…".
func Printable(s string, limit int) string {
	var b strings.Builder
	n := 0
	for i, r := range s {
		if n == limit {
			b.WriteString("…")
			break
		}
		switch {
		case r == utf8.RuneError && !strings.HasPrefix(s[i:], "�"):
			b.WriteString(`\x` + strconv.FormatUint(uint64(s[i]), 16))
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			b.WriteString(`\u` + leftPad(strconv.FormatUint(uint64(r), 16), 4))
		default:
			b.WriteRune(r)
		}
		n++
	}
	return b.String()
}

func leftPad(s string, width int) string {
	return strings.Repeat("0", max(width-len(s), 0)) + s
}
