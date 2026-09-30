package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
)

const CookieName = "did"

// Sign returns did.hex(hmac). The did itself must not contain a dot.
func Sign(secret, did string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(did))
	return did + "." + hex.EncodeToString(mac.Sum(nil))
}

// Parse checks the signature and returns the device id.
func Parse(secret, token string) (string, error) {
	did, sig, ok := strings.Cut(token, ".")
	if !ok || did == "" || sig == "" {
		return "", errors.New("malformed")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(did))
	want := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(sig), []byte(want)) != 1 {
		return "", errors.New("bad signature")
	}
	return did, nil
}

// FromRequest reads the did cookie.
func FromRequest(r *http.Request, secret string) (string, error) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return "", err
	}
	return Parse(secret, c.Value)
}

// SetCookie writes the device cookie. Secure is off for local HTTP.
func SetCookie(w http.ResponseWriter, secret, did string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    Sign(secret, did),
		Path:     "/",
		MaxAge:   365 * 24 * 3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}
