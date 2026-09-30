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
)

func main() {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		log.Fatal(err)
	}
	o := &orch.Orch{
		RDB:   live.Open(),
		Proxy: serve.Env("PROXY_URL", "http://127.0.0.1:8081"),
		Token: serve.Env("INTERNAL_TOKEN", ""),
		ID:    hex.EncodeToString(buf),
		HTTP:  &http.Client{Timeout: 3 * time.Second},
	}
	ctx := context.Background()
	for {
		if err := o.Tick(ctx); err != nil {
			log.Printf("tick: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
