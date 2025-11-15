package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

var hmacSecret = []byte("c04b0c57-bffc-4b22-84e7-e70d3bba11bb")

func randomToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func sign(value string) string {
	h := hmac.New(sha256.New, hmacSecret)
	h.Write([]byte(value))
	s := h.Sum(nil)
	return value + "." + base64.RawURLEncoding.EncodeToString(s)
}

func verifySigned(signed string) bool {
	parts := strings.Split(signed, ".")
	if len(parts) != 2 {
		return false
	}
	value := parts[0]
	sig, _ := base64.RawURLEncoding.DecodeString(parts[1])

	h := hmac.New(sha256.New, hmacSecret)
	h.Write([]byte(value))
	expected := h.Sum(nil)

	return hmac.Equal(sig, expected)
}
