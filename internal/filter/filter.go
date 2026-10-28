package filter

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"colorbook/internal/words"
)

// Filter is an in-memory whole-token blocklist.
// The ballot keeps the original text. This checks the letter core.
type Filter struct {
	exact    map[string]struct{}
	skeleton map[string]struct{}
	allow    map[string]struct{}
}

// Reject reports whether the original submission is blocked.
// Punctuation-only text has an empty letter core and is allowed.
func (f *Filter) Reject(original string) bool {
	core := words.LetterCore(original)
	if core == "" {
		return false
	}
	if _, ok := f.exact[core]; ok {
		return true
	}
	if _, ok := f.allow[core]; ok {
		return false
	}
	// Short skeletons (e.g. "c" from choochi) collide with ordinary English
	// like ice/ace/cook. Require enough consonants to stay discriminative.
	sk := Skeleton(core)
	if len(sk) < 3 {
		return false
	}
	if _, ok := f.skeleton[sk]; ok {
		return true
	}
	return false
}

// AddExact is for tests.
func (f *Filter) AddExact(core string) {
	if f.exact == nil {
		f.exact = map[string]struct{}{}
	}
	f.exact[words.LetterCore(core)] = struct{}{}
}

// AddSkeletonLemma stores the phonetic skeleton of a lemma.
func (f *Filter) AddSkeletonLemma(lemma string) {
	if f.skeleton == nil {
		f.skeleton = map[string]struct{}{}
	}
	f.skeleton[Skeleton(words.LetterCore(lemma))] = struct{}{}
}

// Allow skips the phonetic tier for an ordinary English word.
func (f *Filter) Allow(core string) {
	if f.allow == nil {
		f.allow = map[string]struct{}{}
	}
	f.allow[words.LetterCore(core)] = struct{}{}
}

// Skeleton collapses Indic romanization: aspiration, vowel length, and w/v.
// Vowels and h are dropped and repeats collapse, so unseen spellings share a key.
func Skeleton(s string) string {
	var b strings.Builder
	var prev rune
	for _, r := range s {
		if r < 'a' || r > 'z' {
			continue
		}
		if r == 'w' {
			r = 'v'
		}
		if r == 'h' || r == 'a' || r == 'e' || r == 'i' || r == 'o' || r == 'u' {
			continue
		}
		if r == prev {
			continue
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}

// Load reads exact/*.txt, indic.txt, and allow.txt.
// exact/hi.txt lemmas of length >= 6 also feed the phonetic tier.
// allow.txt holds everyday English letter cores so common words skip that tier.
func Load(dir string) (*Filter, error) {
	f := &Filter{
		exact:    map[string]struct{}{},
		skeleton: map[string]struct{}{},
		allow:    map[string]struct{}{},
	}
	exactDir := filepath.Join(dir, "exact")
	entries, err := os.ReadDir(exactDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		path := filepath.Join(exactDir, e.Name())
		lines, err := readLines(path)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			core := words.LetterCore(line)
			if len(core) < 3 {
				continue
			}
			f.exact[core] = struct{}{}
			if e.Name() == "hi.txt" && len(core) >= 6 {
				f.skeleton[Skeleton(core)] = struct{}{}
			}
		}
	}
	indic, err := readLines(filepath.Join(dir, "indic.txt"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, line := range indic {
		core := words.LetterCore(line)
		if len(core) >= 6 {
			f.skeleton[Skeleton(core)] = struct{}{}
		}
	}
	allow, err := readLines(filepath.Join(dir, "allow.txt"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, line := range allow {
		core := words.LetterCore(line)
		if core != "" {
			f.allow[core] = struct{}{}
		}
	}
	return f, nil
}

func readLines(path string) ([]string, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	var out []string
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, sc.Err()
}
