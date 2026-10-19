package main

import (
	"context"
	"log"
	"net/http"

	"colorbook/internal/canvas"
	"colorbook/internal/live"
	"colorbook/internal/pgstore"
	"colorbook/internal/serve"
	"colorbook/internal/svcjwt"
)

func main() {
	store, err := pgstore.Open(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	pubs, err := svcjwt.LoadPubs()
	if err != nil {
		log.Fatal(err)
	}
	s := &canvas.Service{Store: store, RDB: live.Open(), OrchPub: pubs.Orch}
	mux := http.NewServeMux()
	s.Register(mux)
	serve.Listen(mux)
}
