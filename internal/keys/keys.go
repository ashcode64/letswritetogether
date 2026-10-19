package keys

import "fmt"

const (
	PhaseKey = "phase"
	LeaseKey = "lease"
	Channel  = "live"
	TTL      = 180
)

func Dedupe(round int64) string { return fmt.Sprintf("dedupe:%d", round) }
func Submit(round int64, did string) string {
	return fmt.Sprintf("submit:%d:%s", round, did)
}
func Words(round int64) string { return fmt.Sprintf("words:%d", round) }
func Order(round int64) string { return fmt.Sprintf("order:%d", round) }
func Seq(round int64) string   { return fmt.Sprintf("wordseq:%d", round) }
func Vote(round int64, did string) string {
	return fmt.Sprintf("vote:%d:%s", round, did)
}
func Tally(round int64) string      { return fmt.Sprintf("tally:%d", round) }
func Closed(round int64) string     { return fmt.Sprintf("closed:%d", round) }
func Device(did string) string      { return "device:" + did }
func Refresh(sum string) string     { return "refresh:" + sum }
func RefreshUsed(sum string) string { return "refreshused:" + sum }
