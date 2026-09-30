package clock

import "testing"

func TestPhaseBoundaries(t *testing.T) {
	cases := []struct {
		sec   int64
		phase string
		round int64
	}{
		{29, "submit", 0},
		{30, "vote", 0},
		{59, "vote", 0},
		{60, "submit", 1},
		{90, "vote", 1},
	}
	for _, tc := range cases {
		s := From(tc.sec, 0)
		if s.Phase != tc.phase || s.RoundID != tc.round {
			t.Fatalf("sec %d: got phase %s round %d", tc.sec, s.Phase, s.RoundID)
		}
	}
	if From(29, 0).SubmitMSLeft != 1000 {
		t.Fatalf("submit left: %d", From(29, 0).SubmitMSLeft)
	}
	if From(30, 0).SubmitMSLeft != 0 {
		t.Fatal("submit should be closed at second 30")
	}
	if From(59, 0).VoteMSLeft != 1000 {
		t.Fatalf("vote left: %d", From(59, 0).VoteMSLeft)
	}
}
