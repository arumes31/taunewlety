package pkg

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"strings"
)

// unsubscribeTokenLabel separates unsubscribe tokens from any other use of
// the same secret (for example session cookie signing).
const unsubscribeTokenLabel = "taunewlety-unsubscribe-v1:" // #nosec G101 -- domain-separation label, not a credential

// UnsubscribeSecret returns the key used to sign unsubscribe links:
// UNSUBSCRIBE_SECRET when set, otherwise SESSION_SECRET. A dedicated secret
// lets operators rotate SESSION_SECRET without invalidating the links in
// newsletters that were already sent.
func UnsubscribeSecret() string {
	if s := os.Getenv("UNSUBSCRIBE_SECRET"); s != "" {
		return s
	}
	return os.Getenv("SESSION_SECRET")
}

// UnsubscribeToken returns the HMAC-SHA256 token that authorizes
// unsubscribing email. It returns "" when no secret is configured, so an
// unsigned (forgeable) token is never produced.
func UnsubscribeToken(secret, email string) string {
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(unsubscribeTokenLabel + normalizeUnsubscribeEmail(email)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyUnsubscribeToken reports whether token is the valid unsubscribe token
// for email, comparing in constant time.
func VerifyUnsubscribeToken(secret, email, token string) bool {
	if secret == "" || email == "" || token == "" {
		return false
	}
	expected := UnsubscribeToken(secret, email)
	return hmac.Equal([]byte(expected), []byte(token))
}

// normalizeUnsubscribeEmail makes the token independent of letter case and
// surrounding whitespace, so a link still verifies if it is re-cased in transit.
func normalizeUnsubscribeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
