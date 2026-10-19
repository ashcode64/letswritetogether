package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
)

// JWK is a P-256 public key. The private key never reaches the server.
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (j JWK) canonical() string {
	return fmt.Sprintf(`{"crv":"P-256","kty":"EC","x":%q,"y":%q}`, j.X, j.Y)
}

// Thumbprint is the hex SHA-256 of the canonical public key. It is the device id.
func Thumbprint(j JWK) (string, error) {
	if j.Kty != "EC" || j.Crv != "P-256" || j.X == "" || j.Y == "" {
		return "", errors.New("bad key")
	}
	sum := sha256.Sum256([]byte(j.canonical()))
	return hex.EncodeToString(sum[:]), nil
}

func ParseP256(j JWK) (*ecdsa.PublicKey, error) {
	if _, err := Thumbprint(j); err != nil {
		return nil, err
	}
	xb, err := base64.RawURLEncoding.DecodeString(j.X)
	if err != nil || len(xb) != 32 {
		return nil, errors.New("bad key")
	}
	yb, err := base64.RawURLEncoding.DecodeString(j.Y)
	if err != nil || len(yb) != 32 {
		return nil, errors.New("bad key")
	}
	x := new(big.Int).SetBytes(xb)
	y := new(big.Int).SetBytes(yb)
	if !elliptic.P256().IsOnCurve(x, y) {
		return nil, errors.New("bad key")
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
}

// SignP256 returns a SHA-256 ECDSA signature in raw r||s form.
func SignP256(priv *ecdsa.PrivateKey, msg string) (string, error) {
	sum := sha256.Sum256([]byte(msg))
	r, s, err := ecdsa.Sign(rand.Reader, priv, sum[:])
	if err != nil {
		return "", err
	}
	raw := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// VerifyP256 checks a SHA-256 ECDSA signature in raw r||s form, which is what WebCrypto returns.
func VerifyP256(pub *ecdsa.PublicKey, msg string, proof string) bool {
	sig, err := base64.RawURLEncoding.DecodeString(proof)
	if err != nil || len(sig) != 64 {
		return false
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	sum := sha256.Sum256([]byte(msg))
	return ecdsa.Verify(pub, sum[:], r, s)
}

func PublicJWK(pub *ecdsa.PublicKey) JWK {
	xb := pub.X.FillBytes(make([]byte, 32))
	yb := pub.Y.FillBytes(make([]byte, 32))
	return JWK{
		Kty: "EC",
		Crv: "P-256",
		X:   base64.RawURLEncoding.EncodeToString(xb),
		Y:   base64.RawURLEncoding.EncodeToString(yb),
	}
}
