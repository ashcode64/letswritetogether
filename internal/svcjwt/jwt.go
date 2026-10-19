package svcjwt

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	RoleGateway      = "gateway"
	RoleOrchestrator = "orchestrator"
	RoleDevice       = "device"

	AudGateway    = "gateway"
	AudSubmission = "submission"
	AudVoting     = "voting"
	AudCanvas     = "canvas"
	AudArchive    = "archive"
	AudRealtime   = "realtime"
)

// Claims is a short-lived service or device token.
// A gateway key cannot mint an orchestrator token, because each role has its own key.
type Claims struct {
	Aud  string `json:"aud"`
	Exp  int64  `json:"exp"`
	Iat  int64  `json:"iat"`
	Role string `json:"role"`
	Sub  string `json:"sub,omitempty"`
}

type Pubs struct {
	Gateway ed25519.PublicKey
	Orch    ed25519.PublicKey
}

var header = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"EdDSA","typ":"JWT"}`))

func Sign(priv ed25519.PrivateKey, c Claims) (string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	body := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	sig := ed25519.Sign(priv, []byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func Verify(pub ed25519.PublicKey, token string, now time.Time) (Claims, error) {
	var c Claims
	h, p, s, ok := split(token)
	if !ok || h != header {
		return c, errors.New("malformed")
	}
	sig, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return c, errors.New("malformed")
	}
	if !ed25519.Verify(pub, []byte(h+"."+p), sig) {
		return c, errors.New("bad signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil || json.Unmarshal(raw, &c) != nil {
		return c, errors.New("malformed")
	}
	unix := now.Unix()
	if c.Exp <= unix || c.Iat > unix+5 || c.Exp-c.Iat > 600 {
		return c, errors.New("expired")
	}
	return c, nil
}

// Require verifies a bearer token with the key for the required role.
func Subject(r *http.Request, pub ed25519.PublicKey, aud, role string) (string, error) {
	c, err := Require(r.Header.Get("Authorization"), pub, aud, role, time.Now())
	if err != nil {
		return "", err
	}
	if role == RoleGateway && c.Sub == "" {
		return "", errors.New("missing subject")
	}
	return c.Sub, nil
}

func Require(auth string, pub ed25519.PublicKey, aud, role string, now time.Time) (Claims, error) {
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == "" || token == auth {
		return Claims{}, errors.New("missing")
	}
	c, err := Verify(pub, token, now)
	if err != nil {
		return Claims{}, err
	}
	if c.Aud != aud || c.Role != role {
		return Claims{}, errors.New("rejected")
	}
	return c, nil
}

func split(token string) (string, string, string, bool) {
	h, rest, ok := strings.Cut(token, ".")
	if !ok {
		return "", "", "", false
	}
	p, s, ok := strings.Cut(rest, ".")
	if !ok || strings.Contains(s, ".") {
		return "", "", "", false
	}
	return h, p, s, true
}

func PrivateFromEnv(name string) (ed25519.PrivateKey, error) {
	seed, err := decodeKey(os.Getenv(name))
	if err != nil {
		return nil, err
	}
	if len(seed) != ed25519.SeedSize {
		return nil, errors.New(name + " must be a 32-byte seed")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func PublicFromEnv(name string) (ed25519.PublicKey, error) {
	raw, err := decodeKey(os.Getenv(name))
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, errors.New(name + " must be a 32-byte public key")
	}
	return ed25519.PublicKey(raw), nil
}

func LoadPubs() (Pubs, error) {
	g, err := PublicFromEnv("GATEWAY_PUBLIC_KEY")
	if err != nil {
		return Pubs{}, err
	}
	o, err := PublicFromEnv("ORCH_PUBLIC_KEY")
	if err != nil {
		return Pubs{}, err
	}
	return Pubs{Gateway: g, Orch: o}, nil
}

func decodeKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("missing key")
	}
	return hex.DecodeString(s)
}
