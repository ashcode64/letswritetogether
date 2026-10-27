package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"time"

	"colorbook/internal/live"
	"colorbook/internal/orch"
	"colorbook/internal/serve"
	"colorbook/internal/svcjwt"
)

func main() {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		log.Fatal(err)
	}
	signer, err := svcjwt.PrivateFromEnv("ORCH_PRIVATE_KEY")
	if err != nil {
		log.Fatal(err)
	}
	o := &orch.Orch{
		RDB:       live.Open(),
		Proxy:     serve.Env("PROXY_URL", "http://127.0.0.1:8081"),
		ProxyHost: serve.Env("PROXY_HOST", ""),
		Signer:    signer,
		ID:        hex.EncodeToString(buf),
		HTTP:      &http.Client{Timeout: 3 * time.Second},
	}
	ctx := context.Background()
	for {
		if err := o.Tick(ctx); err != nil {
			log.Printf("tick: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
