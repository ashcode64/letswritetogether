package main

import (
	"log"
	"net/http"

	"colorbook/internal/live"
	"colorbook/internal/serve"
	"colorbook/internal/svcjwt"
	"colorbook/internal/voting"
)

func main() {
	pubs, err := svcjwt.LoadPubs()
	if err != nil {
		log.Fatal(err)
	}
	s := &voting.Service{RDB: live.Open(), GatewayPub: pubs.Gateway, OrchPub: pubs.Orch}
	mux := http.NewServeMux()
	s.Register(mux)
	serve.Listen(mux)
}
