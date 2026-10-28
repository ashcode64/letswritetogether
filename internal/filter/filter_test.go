package filter

import "testing"

func TestRejectAndAllow(t *testing.T) {
	f := &Filter{}
	f.AddExact("badword")
	f.Allow("classic")
	f.AddSkeletonLemma("bhenchod")
	// classic's skeleton collides with a planted lemma only if we add it.
	f.skeleton[Skeleton("classic")] = struct{}{}

	if !f.Reject("BADWORD") || !f.Reject("badword!") || !f.Reject("b a d w o r d") {
		t.Fatal("letter core should catch case, punctuation, and spaces")
	}
	if f.Reject("helo") {
		t.Fatal("one-letter variant must be allowed")
	}
	if f.Reject("!") || f.Reject("...") {
		t.Fatal("punctuation-only is allowed")
	}
	if f.Reject("classic") {
		t.Fatal("allowlist must skip the phonetic tier")
	}
	if !f.Reject("clascic") && Skeleton("clascic") == Skeleton("classic") {
		t.Fatal("non-allow word sharing the skeleton should be rejected")
	}
	if Skeleton("bhenchod") != Skeleton("bahanchod") || Skeleton("bhenchod") != Skeleton("bhainchod") {
		t.Fatalf("skeletons %s %s %s", Skeleton("bhenchod"), Skeleton("bahanchod"), Skeleton("bhainchod"))
	}
	if !f.Reject("bahanchod") {
		t.Fatal("unseen indic spelling should hit the skeleton")
	}
}

func TestLoadLists(t *testing.T) {
	f, err := Load("../../data/blocklist")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.exact) < 100 {
		t.Fatalf("expected latin lists, got %d", len(f.exact))
	}
	if len(f.allow) < 1000 {
		t.Fatalf("expected everyday allowlist, got %d", len(f.allow))
	}
	for _, w := range []string{"classic", "ice", "cook", "city", "each", "choice", "coach", "water", "hello"} {
		if f.Reject(w) {
			t.Fatalf("%q must survive the shipped lists", w)
		}
	}
	if !f.Reject("sharmuta") || !f.Reject("Sharmuta!") || !f.Reject("caonima") {
		t.Fatal("arabizi and hand-list lemmas must reject")
	}
	if !f.Reject("choochi") || !f.Reject("chutiya") {
		t.Fatal("exact Indic lemmas must still reject")
	}
}

func TestShortSkeletonDoesNotBlock(t *testing.T) {
	f := &Filter{}
	f.skeleton["c"] = struct{}{}
	if f.Reject("ice") || f.Reject("ace") {
		t.Fatal("skeleton keys shorter than 3 must not reject English")
	}
	f.skeleton["cty"] = struct{}{}
	if !f.Reject("chutya") {
		t.Fatal("skeleton length >= 3 should still reject")
	}
}
