package main

import (
	"context"
	"log"
	"net/http"

	"colorbook/internal/canvas"
	"colorbook/internal/live"
	"colorbook/internal/pgstore"
	"colorbook/internal/serve"
)

func main() {
	store, err := pgstore.Open(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	s := &canvas.Service{Store: store, RDB: live.Open(), Token: serve.Env("INTERNAL_TOKEN", "")}
	mux := http.NewServeMux()
	s.Register(mux)
	serve.Listen(mux)
}
