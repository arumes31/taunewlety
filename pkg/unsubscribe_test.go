package pkg

import (
	"testing"
)

func TestUnsubscribeToken(t *testing.T) {
	const secret = "test-secret"

	token := UnsubscribeToken(secret, "user@example.com")
	if token == "" {
		t.Fatal("expected a token")
	}
	last := byte('A')
	if token[len(token)-1] == last {
		last = 'B'
	}
	tampered := token[:len(token)-1] + string(last)

	tests := []struct {
		name   string
		secret string
		email  string
		token  string
		want   bool
	}{
		{"valid token", secret, "user@example.com", token, true},
		{"case and whitespace insensitive", secret, " User@Example.COM ", token, true},
		{"other address", secret, "other@example.com", token, false},
		{"other secret", "another-secret", "user@example.com", token, false},
		{"tampered token", secret, "user@example.com", tampered, false},
		{"empty token", secret, "user@example.com", "", false},
		{"empty email", secret, "", token, false},
		{"no secret configured", "", "user@example.com", UnsubscribeToken("", "user@example.com"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifyUnsubscribeToken(tt.secret, tt.email, tt.token); got != tt.want {
				t.Errorf("VerifyUnsubscribeToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnsubscribeToken_EmptySecret(t *testing.T) {
	if got := UnsubscribeToken("", "user@example.com"); got != "" {
		t.Errorf("expected no token without a secret, got %q", got)
	}
}

func TestUnsubscribeSecret(t *testing.T) {
	t.Setenv("SESSION_SECRET", "session")
	t.Setenv("UNSUBSCRIBE_SECRET", "")
	if got := UnsubscribeSecret(); got != "session" {
		t.Errorf("expected fallback to SESSION_SECRET, got %q", got)
	}

	t.Setenv("UNSUBSCRIBE_SECRET", "dedicated")
	if got := UnsubscribeSecret(); got != "dedicated" {
		t.Errorf("expected UNSUBSCRIBE_SECRET, got %q", got)
	}
}
