package main

import (
	"log"
	"net/http"

	"colorbook/internal/filter"
	"colorbook/internal/live"
	"colorbook/internal/serve"
	"colorbook/internal/submit"
)

func main() {
	f, err := filter.Load(serve.Env("BLOCKLIST", "data/blocklist"))
	if err != nil {
		log.Fatal(err)
	}
	s := &submit.Service{RDB: live.Open(), Filter: f}
	mux := http.NewServeMux()
	s.Register(mux)
	serve.Listen(mux)
}
