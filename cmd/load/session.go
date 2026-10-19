package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"colorbook/internal/auth"
)

const dummyTurnstile = "XXXX.DUMMY.TOKEN.XXXX"

func mint(origin string, client *http.Client) (string, error) {
	res, err := client.Post(origin+"/v1/sessions/challenge", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", err
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("challenge: %s %s", res.Status, raw)
	}
	var ch auth.Challenge
	if err := json.Unmarshal(raw, &ch); err != nil {
		return "", err
	}
	n, ok := auth.Solve(ch)
	if !ok {
		return "", fmt.Errorf("proof of work")
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", err
	}
	proof, err := auth.SignP256(priv, "mint\n"+ch.Signature)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]any{
		"public_key":      auth.PublicJWK(&priv.PublicKey),
		"salt":            ch.Salt,
		"challenge":       ch.Challenge,
		"maxnumber":       ch.Max,
		"expires":         ch.Expires,
		"signature":       ch.Signature,
		"solution":        n,
		"proof":           proof,
		"turnstile_token": dummyTurnstile,
	})
	res, err = client.Post(origin+"/v1/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	raw, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("session: %s %s", res.Status, raw)
	}
	var out struct {
		Access string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Access == "" {
		return "", fmt.Errorf("session token")
	}
	return out.Access, nil
}
