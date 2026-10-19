package svcjwt

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

func TestSignVerifyRoleKeys(t *testing.T) {
	_, gw, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, orch, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	tok, err := Sign(gw, Claims{Aud: AudVoting, Exp: now.Unix() + 30, Iat: now.Unix(), Role: RoleGateway, Sub: "device"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := Require("Bearer "+tok, gw.Public().(ed25519.PublicKey), AudVoting, RoleGateway, now)
	if err != nil || c.Sub != "device" {
		t.Fatalf("gateway token: %v %#v", err, c)
	}
	if _, err := Require("Bearer "+tok, orch.Public().(ed25519.PublicKey), AudVoting, RoleOrchestrator, now); err == nil {
		t.Fatal("gateway key must not pass as orchestrator")
	}
	tallied, err := Sign(orch, Claims{Aud: AudVoting, Exp: now.Unix() + 30, Iat: now.Unix(), Role: RoleOrchestrator})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Require("Bearer "+tallied, orch.Public().(ed25519.PublicKey), AudVoting, RoleOrchestrator, now); err != nil {
		t.Fatal(err)
	}
	if _, err := Require("Bearer "+tallied, gw.Public().(ed25519.PublicKey), AudVoting, RoleGateway, now); err == nil {
		t.Fatal("orchestrator key must not pass as gateway")
	}
	if _, err := Verify(gw.Public().(ed25519.PublicKey), tok, now.Add(time.Minute)); err == nil {
		t.Fatal("expired token accepted")
	}
}
