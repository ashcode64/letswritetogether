package words

import "testing"

func TestValidateExactIdentity(t *testing.T) {
	for _, s := range []string{"hello", "heLLO", "HELLO", "hello!", "hello-", "h e l l o", "hello world", "!", "...", "—"} {
		got, ok := Validate(s)
		if !ok || got != s {
			t.Fatalf("%q -> %q ok=%v", s, got, ok)
		}
	}
	for _, s := range []string{"", "   ", "hello1", "hello@", "café", "hello#"} {
		if _, ok := Validate(s); ok {
			t.Fatalf("expected reject %q", s)
		}
	}
	got, ok := Validate("  hello  ")
	if !ok || got != "hello" {
		t.Fatalf("trim: %q %v", got, ok)
	}
}

func TestLetterCore(t *testing.T) {
	if LetterCore("HELLO") != "hello" || LetterCore("hello!") != "hello" || LetterCore("h e l l o") != "hello" {
		t.Fatal(LetterCore("HELLO"), LetterCore("hello!"), LetterCore("h e l l o"))
	}
	if LetterCore("!") != "" || LetterCore("...") != "" {
		t.Fatal("punctuation should have an empty letter core")
	}
}
