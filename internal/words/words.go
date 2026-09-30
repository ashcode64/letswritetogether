package words

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// prose marks allowed anywhere in a submission.
const prose = ".,!?;:'\"-—–…()[]{}‘’“”&/"

// Validate trims only the ends. Internal spaces and essay punctuation stay.
// Digits and other symbols are rejected. Length is 1–48 runes.
func Validate(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	rs := []rune(s)
	if len(rs) < 1 || len(rs) > 48 {
		return "", false
	}
	okMark := false
	for _, r := range rs {
		switch {
		case r == ' ':
			continue
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			okMark = true
		case strings.ContainsRune(prose, r):
			okMark = true
		default:
			return "", false
		}
	}
	if !okMark {
		return "", false
	}
	return s, true
}

// LetterCore is the blocklist key: casefolded letters only.
// HELLO, hello!, and "h e l l o" all become hello.
func LetterCore(s string) string {
	s = strings.ToLower(fold(s))
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
