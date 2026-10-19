package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strconv"
	"time"
)

// Challenge is a stateless proof-of-work. The answer is not included.
type Challenge struct {
	Salt      string `json:"salt"`
	Challenge string `json:"challenge"`
	Max       int    `json:"maxnumber"`
	Expires   int64  `json:"expires"`
	Signature string `json:"signature"`
}

func NewChallenge(secret string, max int, now time.Time) (Challenge, error) {
	if max < 1 {
		max = 1
	}
	if max > 200000 {
		max = 200000
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return Challenge{}, err
	}
	salt := hex.EncodeToString(buf)
	num := make([]byte, 2)
	if _, err := rand.Read(num); err != nil {
		return Challenge{}, err
	}
	n := (int(num[0])<<8 | int(num[1])) % (max + 1)
	sum := sha256.Sum256([]byte(salt + strconv.Itoa(n)))
	c := Challenge{
		Salt:      salt,
		Challenge: hex.EncodeToString(sum[:]),
		Max:       max,
		Expires:   now.Add(2 * time.Minute).Unix(),
	}
	c.Signature = signChallenge(secret, c)
	return c, nil
}

func CheckChallenge(secret string, c Challenge, solution int, now time.Time) error {
	if c.Max < 1 || c.Max > 200000 || solution < 0 || solution > c.Max {
		return errors.New("bad challenge")
	}
	if now.Unix() > c.Expires {
		return errors.New("expired challenge")
	}
	mac := signChallenge(secret, c)
	if subtle.ConstantTimeCompare([]byte(mac), []byte(c.Signature)) != 1 {
		return errors.New("bad challenge")
	}
	sum := sha256.Sum256([]byte(c.Salt + strconv.Itoa(solution)))
	if hex.EncodeToString(sum[:]) != c.Challenge {
		return errors.New("bad solution")
	}
	return nil
}

func Solve(c Challenge) (int, bool) {
	for n := 0; n <= c.Max; n++ {
		sum := sha256.Sum256([]byte(c.Salt + strconv.Itoa(n)))
		if hex.EncodeToString(sum[:]) == c.Challenge {
			return n, true
		}
	}
	return 0, false
}

func signChallenge(secret string, c Challenge) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(c.Salt + "." + c.Challenge + "." + strconv.FormatInt(c.Expires, 10) + "." + strconv.Itoa(c.Max)))
	return hex.EncodeToString(mac.Sum(nil))
}
