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
	mac.Write([]byte(unsubscribeTokenLabel + CanonicalEmail(email)))
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

// CanonicalEmail is the identity an unsubscribe token is bound to: the address
// with surrounding whitespace removed and the domain lowercased. The local
// part keeps its case because mailboxes may be case-sensitive (RFC 5321), so
// Bob@example.com and bob@example.com are distinct subscribers and must not
// share a token. Callers deleting a subscriber must match on this same form.
func CanonicalEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return email
	}
	return email[:at+1] + strings.ToLower(email[at+1:])
}
