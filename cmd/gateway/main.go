package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"colorbook/internal/gateway"
	"colorbook/internal/live"
	"colorbook/internal/serve"
	"colorbook/internal/svcjwt"
)

func main() {
	priv, err := svcjwt.PrivateFromEnv("GATEWAY_PRIVATE_KEY")
	if err != nil {
		log.Fatal(err)
	}
	pow, _ := strconv.Atoi(os.Getenv("POW_MAX"))
	s := gateway.New(
		live.Open(),
		serve.Env("COOKIE_SECRET", "dev-cookie-secret"),
		serve.Env("PROXY_URL", "http://127.0.0.1:8081"),
		os.Getenv("COOKIE_SECURE") == "1",
		priv,
		os.Getenv("TURNSTILE_SECRET"),
		os.Getenv("TURNSTILE_SITE_KEY"),
		pow,
	)
	mux := http.NewServeMux()
	s.Register(mux)
	serve.Listen(mux)
}
