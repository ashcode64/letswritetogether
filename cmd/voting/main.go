package main

import (
	"net/http"

	"colorbook/internal/live"
	"colorbook/internal/serve"
	"colorbook/internal/voting"
)

func main() {
	s := &voting.Service{RDB: live.Open(), Token: serve.Env("INTERNAL_TOKEN", "")}
	mux := http.NewServeMux()
	s.Register(mux)
	serve.Listen(mux)
}
