package essay

import "testing"

func TestJoin(t *testing.T) {
	if got := Join([]string{"hello", "world"}); got != "hello world" {
		t.Fatal(got)
	}
	if got := Join([]string{"hello", "!"}); got != "hello!" {
		t.Fatal(got)
	}
	if got := Join([]string{"hello", ","}); got != "hello," {
		t.Fatal(got)
	}
	if got := Join([]string{"(", "hello", ")"}); got != "(hello)" {
		t.Fatal(got)
	}
	if got := Join([]string{"Say", `"hello"`}); got != `Say "hello"` {
		t.Fatal(got)
	}
	if got := Join(nil); got != "" {
		t.Fatal(got)
	}
}
