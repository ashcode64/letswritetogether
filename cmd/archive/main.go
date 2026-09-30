package main

import (
	"context"
	"log"
	"net/http"

	"colorbook/internal/archive"
	"colorbook/internal/pgstore"
	"colorbook/internal/serve"
)

func main() {
	store, err := pgstore.Open(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	s := &archive.Service{Store: store, Token: serve.Env("INTERNAL_TOKEN", "")}
	mux := http.NewServeMux()
	s.Register(mux)
	serve.Listen(mux)
}
