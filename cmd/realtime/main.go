package main

import (
	"context"
	"net/http"

	"colorbook/internal/live"
	"colorbook/internal/realtime"
	"colorbook/internal/serve"
)

func main() {
	h := realtime.New(live.Open())
	go h.Listen(context.Background())
	mux := http.NewServeMux()
	h.Register(mux)
	serve.Listen(mux)
}
