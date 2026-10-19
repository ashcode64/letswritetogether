package main

import (
	"context"
	"log"
	"net/http"

	"colorbook/internal/archive"
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
	s := &archive.Service{Store: store, OrchPub: pubs.Orch}
	mux := http.NewServeMux()
	s.Register(mux)
	serve.Listen(mux)
}
