package gateway

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"colorbook/internal/auth"
)

func TestMintRefreshAndReuse(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s := New(rdb, "secret", "http://127.0.0.1", "", false, priv, testTurnstile, "site", 200)
	mux := http.NewServeMux()
	s.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/sessions/challenge", nil))
	if rec.Code != 200 {
		t.Fatalf("challenge %d %s", rec.Code, rec.Body.String())
	}
	var ch auth.Challenge
	if err := json.Unmarshal(rec.Body.Bytes(), &ch); err != nil {
		t.Fatal(err)
	}
	n, ok := auth.Solve(ch)
	if !ok {
		t.Fatal("solve")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := auth.SignP256(key, "mint\n"+ch.Signature)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{
		"public_key": auth.PublicJWK(&key.PublicKey), "salt": ch.Salt, "challenge": ch.Challenge,
		"maxnumber": ch.Max, "expires": ch.Expires, "signature": ch.Signature, "solution": n,
		"proof": proof, "turnstile_token": dummyToken,
	})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(string(raw))))
	if rec.Code != 200 {
		t.Fatalf("mint %d %s", rec.Code, rec.Body.String())
	}
	var minted struct {
		Access string `json:"access_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &minted); err != nil || minted.Access == "" {
		t.Fatal(rec.Body.String())
	}
	cookie := rec.Result().Cookies()[0].Value
	rec = httptest.NewRecorder()
	cur := httptest.NewRequest(http.MethodGet, "/v1/rounds/current", nil)
	cur.Header.Set("Authorization", "Bearer "+minted.Access)
	mux.ServeHTTP(rec, cur)
	if rec.Code != 200 {
		t.Fatalf("current %d %s", rec.Code, rec.Body.String())
	}

	ts := time.Now().Unix()
	refreshProof, err := auth.SignP256(key, "refresh\n"+strconv.FormatInt(ts, 10))
	if err != nil {
		t.Fatal(err)
	}
	body := `{"ts":` + strconv.FormatInt(ts, 10) + `,"proof":"` + refreshProof + `"}`
	rec = postRefresh(mux, cookie, body)
	if rec.Code != 200 {
		t.Fatalf("refresh %d %s", rec.Code, rec.Body.String())
	}
	rec = postRefresh(mux, cookie, body)
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), "DEVICE_REVOKED") {
		t.Fatalf("reuse %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/rounds/current", nil)
	req.Header.Set("Authorization", "Bearer "+minted.Access)
	mux.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("revoked access %d %s", rec.Code, rec.Body.String())
	}
}

func postRefresh(mux *http.ServeMux, cookie, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/refresh", strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: refreshCookie, Value: cookie})
	mux.ServeHTTP(rec, req)
	return rec
}
