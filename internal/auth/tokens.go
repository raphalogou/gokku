package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"strings"
)

// randomToken returns a URL-safe random string of n bytes of entropy.
func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken returns the sha256 hex of a session, invite or API token. Only hashes are stored.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

const (
	apiTokenPrefix  = "gk_"
	apiTokenLength  = 40
	base62Alphabet  = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	displayedPrefix = len(apiTokenPrefix) + 6
)

// newAPIToken returns gk_ followed by 40 base62 characters (AUTH-6).
func newAPIToken() string {
	out := make([]byte, apiTokenLength)
	max := big.NewInt(int64(len(base62Alphabet)))
	for i := range out {
		n, _ := rand.Int(rand.Reader, max) // crypto/rand never fails
		out[i] = base62Alphabet[n.Int64()]
	}
	return apiTokenPrefix + string(out)
}

// looksLikeAPIToken checks the shape before any database lookup.
func looksLikeAPIToken(s string) bool {
	if len(s) != len(apiTokenPrefix)+apiTokenLength || s[:len(apiTokenPrefix)] != apiTokenPrefix {
		return false
	}
	for _, c := range s[len(apiTokenPrefix):] {
		if !strings.ContainsRune(base62Alphabet, c) {
			return false
		}
	}
	return true
}
