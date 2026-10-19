package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"
)

func TestChallengeRoundTrip(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c, err := NewChallenge("secret", 1000, now)
	if err != nil {
		t.Fatal(err)
	}
	n, ok := Solve(c)
	if !ok {
		t.Fatal("unsolved")
	}
	if err := CheckChallenge("secret", c, n, now); err != nil {
		t.Fatal(err)
	}
	if err := CheckChallenge("secret", c, n, now.Add(3*time.Minute)); err == nil {
		t.Fatal("expired challenge accepted")
	}
	c.Signature = "00"
	if err := CheckChallenge("secret", c, n, now); err == nil {
		t.Fatal("bad signature accepted")
	}
}

func TestDeviceProof(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	j := PublicJWK(&priv.PublicKey)
	id, err := Thumbprint(j)
	if err != nil || len(id) != 64 {
		t.Fatalf("thumbprint %q %v", id, err)
	}
	pub, err := ParseP256(j)
	if err != nil {
		t.Fatal(err)
	}
	msg := "mint\nabc"
	sum := sha256.Sum256([]byte(msg))
	r, s, err := ecdsa.Sign(rand.Reader, priv, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	raw := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	proof := base64.RawURLEncoding.EncodeToString(raw)
	if !VerifyP256(pub, msg, proof) {
		t.Fatal("proof rejected")
	}
	if VerifyP256(pub, "other", proof) {
		t.Fatal("proof matched the wrong message")
	}
}
