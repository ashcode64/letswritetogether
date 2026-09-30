package essay

// Join renders winning fragments as one paragraph.
// A normal word gets a single space before it.
// A punctuation-only fragment, or one that starts with a mark that binds
// to the previous word, does not. A fragment after an opening quote or
// parenthesis does not get a space either, so the essay does not read "hello !".
func Join(parts []string) string {
	var b []byte
	for i, p := range parts {
		if i > 0 && needsSpace(parts[i-1], p) {
			b = append(b, ' ')
		}
		b = append(b, p...)
	}
	return string(b)
}

func needsSpace(prev, next string) bool {
	if next == "" {
		return false
	}
	if !hasLetter(next) {
		return false
	}
	r := []rune(next)[0]
	switch r {
	case '.', ',', '!', '?', ';', ':', '…', ')', ']', '}':
		return false
	}
	if prev == "" {
		return true
	}
	last := []rune(prev)
	switch last[len(last)-1] {
	case '(', '[', '{', '"', '\'', '“', '‘':
		return false
	}
	return true
}

func hasLetter(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}
