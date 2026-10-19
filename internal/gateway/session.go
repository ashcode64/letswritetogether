package gateway

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"colorbook/internal/auth"
	"colorbook/internal/keys"
	"colorbook/internal/problem"
	"colorbook/internal/svcjwt"
)

const (
	refreshCookie = "refresh"
	refreshTTL    = 30 * 24 * time.Hour
	usedTTL       = 24 * time.Hour
	accessTTL     = 5 * time.Minute
	testTurnstile = "1x0000000000000000000000000000000AA"
	dummyToken    = "XXXX.DUMMY.TOKEN.XXXX"
)

type mintBody struct {
	PublicKey auth.JWK `json:"public_key"`
	Salt      string   `json:"salt"`
	Challenge string   `json:"challenge"`
	Max       int      `json:"maxnumber"`
	Expires   int64    `json:"expires"`
	Signature string   `json:"signature"`
	Solution  int      `json:"solution"`
	Proof     string   `json:"proof"`
	Turnstile string   `json:"turnstile_token"`
	TS        int64    `json:"ts"`
}

func (s *Service) sessionConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"turnstile_site_key": s.TurnstileSite})
}

func (s *Service) challenge(w http.ResponseWriter, r *http.Request) {
	if !s.lim.allow(s.clientIP(r)+":challenge", 20, time.Minute) {
		w.Header().Set("Retry-After", "2")
		problem.Write(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many challenges")
		return
	}
	c, err := auth.NewChallenge(s.Secret, s.PowMax, time.Now())
	if err != nil {
		problem.Write(w, http.StatusInternalServerError, "UNAVAILABLE", "could not mint a challenge")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Service) session(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	if !s.lim.allow(ip+":session", 10, time.Minute) {
		w.Header().Set("Retry-After", "2")
		problem.Write(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many new sessions")
		return
	}
	var body mintBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
		problem.Write(w, http.StatusBadRequest, "BAD_WORD", "malformed session")
		return
	}
	ch := auth.Challenge{Salt: body.Salt, Challenge: body.Challenge, Max: body.Max, Expires: body.Expires, Signature: body.Signature}
	if err := auth.CheckChallenge(s.Secret, ch, body.Solution, time.Now()); err != nil {
		problem.Write(w, http.StatusBadRequest, "VERIFICATION_REQUIRED", "proof of work failed")
		return
	}
	did, err := auth.Thumbprint(body.PublicKey)
	if err != nil {
		problem.Write(w, http.StatusBadRequest, "BAD_WORD", "device key is not valid")
		return
	}
	pub, err := auth.ParseP256(body.PublicKey)
	if err != nil || !auth.VerifyP256(pub, "mint\n"+body.Signature, body.Proof) {
		problem.Write(w, http.StatusBadRequest, "VERIFICATION_REQUIRED", "device proof failed")
		return
	}
	if err := s.checkHuman(r.Context(), body.Turnstile, ip); err != nil {
		problem.Write(w, http.StatusForbidden, "VERIFICATION_REQUIRED", "human check failed")
		return
	}
	revoked, err := s.revoked(r.Context(), did)
	if err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
		return
	}
	if revoked {
		problem.Write(w, http.StatusUnauthorized, "DEVICE_REVOKED", "this device was revoked")
		return
	}
	raw, _ := json.Marshal(body.PublicKey)
	if err := s.RDB.HSet(r.Context(), keys.Device(did), "jwk", string(raw), "revoked", "0").Err(); err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
		return
	}
	if err := s.issue(w, r, did); err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "could not mint a session")
	}
}

func (s *Service) refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(refreshCookie)
	if err != nil || c.Value == "" {
		problem.Write(w, http.StatusUnauthorized, "SESSION_REQUIRED", "session required")
		return
	}
	var body mintBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
		problem.Write(w, http.StatusBadRequest, "BAD_WORD", "malformed session")
		return
	}
	if body.TS < time.Now().Unix()-120 || body.TS > time.Now().Unix()+5 {
		problem.Write(w, http.StatusUnauthorized, "SESSION_REQUIRED", "session required")
		return
	}
	sum := hashToken(c.Value)
	did, err := s.RDB.Get(r.Context(), keys.Refresh(sum)).Result()
	if err == redis.Nil {
		s.reuse(w, r, sum)
		return
	}
	if err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
		return
	}
	revoked, err := s.revoked(r.Context(), did)
	if err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
		return
	}
	if revoked {
		problem.Write(w, http.StatusUnauthorized, "DEVICE_REVOKED", "this device was revoked")
		return
	}
	raw, err := s.RDB.HGet(r.Context(), keys.Device(did), "jwk").Result()
	if err != nil {
		problem.Write(w, http.StatusUnauthorized, "SESSION_REQUIRED", "session required")
		return
	}
	var jwk auth.JWK
	if json.Unmarshal([]byte(raw), &jwk) != nil {
		problem.Write(w, http.StatusUnauthorized, "SESSION_REQUIRED", "session required")
		return
	}
	pub, err := auth.ParseP256(jwk)
	if err != nil || !auth.VerifyP256(pub, "refresh\n"+strconv.FormatInt(body.TS, 10), body.Proof) {
		problem.Write(w, http.StatusUnauthorized, "SESSION_REQUIRED", "session required")
		return
	}
	if err := s.RDB.Del(r.Context(), keys.Refresh(sum)).Err(); err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store unavailable")
		return
	}
	_ = s.RDB.Set(r.Context(), keys.RefreshUsed(sum), did, usedTTL).Err()
	if err := s.issue(w, r, did); err != nil {
		problem.Write(w, http.StatusServiceUnavailable, "UNAVAILABLE", "could not mint a session")
	}
}

func (s *Service) reuse(w http.ResponseWriter, r *http.Request, sum string) {
	did, err := s.RDB.Get(r.Context(), keys.RefreshUsed(sum)).Result()
	if err == nil && did != "" {
		_ = s.RDB.HSet(r.Context(), keys.Device(did), "revoked", "1").Err()
		problem.Write(w, http.StatusUnauthorized, "DEVICE_REVOKED", "this device was revoked")
		return
	}
	problem.Write(w, http.StatusUnauthorized, "SESSION_REQUIRED", "session required")
}

func (s *Service) issue(w http.ResponseWriter, r *http.Request, did string) error {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	token := hex.EncodeToString(buf)
	if err := s.RDB.Set(r.Context(), keys.Refresh(hashToken(token)), did, refreshTTL).Err(); err != nil {
		return err
	}
	now := time.Now()
	access, err := svcjwt.Sign(s.Priv, svcjwt.Claims{
		Aud: svcjwt.AudGateway, Role: svcjwt.RoleDevice, Sub: did,
		Iat: now.Unix(), Exp: now.Add(accessTTL).Unix(),
	})
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: token, Path: "/v1/sessions",
		MaxAge: int(refreshTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.Secure,
	})
	writeJSON(w, http.StatusOK, map[string]any{"access_token": access, "expires_in": int(accessTTL.Seconds())})
	return nil
}

func (s *Service) device(r *http.Request) (string, error) {
	pub := s.Priv.Public().(ed25519.PublicKey)
	c, err := svcjwt.Require(r.Header.Get("Authorization"), pub, svcjwt.AudGateway, svcjwt.RoleDevice, time.Now())
	if err != nil || c.Sub == "" {
		return "", errString("session required")
	}
	revoked, err := s.revoked(r.Context(), c.Sub)
	if err != nil {
		return "", err
	}
	if revoked {
		return "", errString("revoked")
	}
	return c.Sub, nil
}

func (s *Service) revoked(ctx context.Context, did string) (bool, error) {
	v, err := s.RDB.HGet(ctx, keys.Device(did), "revoked").Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return v == "1", nil
}

func (s *Service) checkHuman(ctx context.Context, token, ip string) error {
	if token == "" {
		return errString("human check failed")
	}
	if s.TurnstileSecret == testTurnstile {
		if token == dummyToken {
			return nil
		}
		return errString("human check failed")
	}
	form := url.Values{}
	form.Set("secret", s.TurnstileSecret)
	form.Set("response", token)
	form.Set("remoteip", ip)
	form.Set("idempotency_key", hashToken(token + ip)[:32])
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
	var out struct {
		Success bool `json:"success"`
	}
	if json.Unmarshal(raw, &out) != nil || !out.Success {
		return errString("human check failed")
	}
	return nil
}

func (s *Service) bearer(aud, sub string) (string, error) {
	now := time.Now()
	return svcjwt.Sign(s.Priv, svcjwt.Claims{
		Aud: aud, Role: svcjwt.RoleGateway, Sub: sub,
		Iat: now.Unix(), Exp: now.Add(30 * time.Second).Unix(),
	})
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type errString string

func (e errString) Error() string { return string(e) }
