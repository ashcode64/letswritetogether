package main

import (
	"net/http"
	"os"

	"colorbook/internal/gateway"
	"colorbook/internal/live"
	"colorbook/internal/serve"
)

func main() {
	s := gateway.New(
		live.Open(),
		serve.Env("COOKIE_SECRET", "dev-cookie-secret"),
		serve.Env("PROXY_URL", "http://127.0.0.1:8081"),
		os.Getenv("COOKIE_SECURE") == "1",
	)
	mux := http.NewServeMux()
	s.Register(mux)
	serve.Listen(mux)
}
