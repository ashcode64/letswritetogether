package clock

// Snapshot is the server clock for one instant. Redis TIME is the only source.
type Snapshot struct {
	RoundID      int64  `json:"round_id"`
	Phase        string `json:"phase"`
	ServerTime   int64  `json:"server_time"`
	ServerTimeMS int64  `json:"server_time_ms"`
	SubmitMSLeft int64  `json:"submit_ms_left"`
	VoteMSLeft   int64  `json:"vote_ms_left"`
}

// From converts Redis TIME (seconds, microseconds) into the round phase.
// Submission is open while second%60 < 30. Voting stays open the whole minute.
func From(sec, usec int64) Snapshot {
	within := sec % 60
	msInto := usec / 1000
	phase := "vote"
	var submitLeft int64
	if within < 30 {
		phase = "submit"
		submitLeft = (30-within)*1000 - msInto
		if submitLeft < 0 {
			submitLeft = 0
		}
	}
	voteLeft := (60-within)*1000 - msInto
	if voteLeft < 0 {
		voteLeft = 0
	}
	return Snapshot{
		RoundID:      sec / 60,
		Phase:        phase,
		ServerTime:   sec,
		ServerTimeMS: sec*1000 + msInto,
		SubmitMSLeft: submitLeft,
		VoteMSLeft:   voteLeft,
	}
}
