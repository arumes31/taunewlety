package clients

import (
	"errors"
	"strings"
	"testing"
)

const secretAPIKey = "s3cr3t-tautulli-api-key"

// TestTautulliClient_ErrorsRedactAPIKey verifies that a transport error, which
// wraps the full request URL, never carries the API key to a caller or a log.
func TestTautulliClient_ErrorsRedactAPIKey(t *testing.T) {
	// A host that cannot resolve forces a *url.Error wrapping the full URL.
	client := NewTautulliClient("http://taunewlety-invalid-host.invalid", secretAPIKey)

	calls := []struct {
		name string
		run  func() error
	}{
		{"GetRecentlyAdded", func() error { _, err := client.GetRecentlyAdded(1); return err }},
		{"GetWatchHistory", func() error { _, err := client.GetWatchHistory("1"); return err }},
		{"GetTopWatched", func() error { _, err := client.GetTopWatched(1); return err }},
		{"GetTopGenres", func() error { _, err := client.GetTopGenres(1); return err }},
		{"GetHomeStatsAll", func() error { _, err := client.GetHomeStatsAll(1); return err }},
	}

	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			err := c.run()
			if err == nil {
				t.Fatal("expected a transport error")
			}
			if strings.Contains(err.Error(), secretAPIKey) {
				t.Errorf("API key leaked in error: %v", err)
			}
			if !strings.Contains(err.Error(), "[REDACTED]") {
				t.Errorf("expected the key to be replaced with a redaction marker, got: %v", err)
			}
		})
	}
}

func TestTautulliClient_RedactErr(t *testing.T) {
	client := NewTautulliClient("http://example.com", secretAPIKey)

	t.Run("Nil error passes through", func(t *testing.T) {
		if got := client.redactErr(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("Unrelated error is returned unchanged", func(t *testing.T) {
		original := errors.New("connection refused")
		if got := client.redactErr(original); got != original {
			t.Errorf("expected the original error instance, got %v", got)
		}
	})

	t.Run("Empty API key is a no-op", func(t *testing.T) {
		bare := NewTautulliClient("http://example.com", "")
		original := errors.New("some error")
		if got := bare.redactErr(original); got != original {
			t.Errorf("expected the original error instance, got %v", got)
		}
	})

	t.Run("Key is replaced", func(t *testing.T) {
		got := client.redactErr(errors.New("GET http://x/api/v2?apikey=" + secretAPIKey + " failed"))
		if strings.Contains(got.Error(), secretAPIKey) {
			t.Errorf("API key survived redaction: %v", got)
		}
		if !strings.Contains(got.Error(), "[REDACTED]") {
			t.Errorf("expected a redaction marker, got: %v", got)
		}
	})
}

func TestMediaTypeForStat(t *testing.T) {
	tests := map[string]string{
		"top_movies": "movie",
		"top_tv":     "show",
		"top_genres": "",
		"other":      "",
		"":           "",
	}
	for statID, want := range tests {
		if got := mediaTypeForStat(statID); got != want {
			t.Errorf("mediaTypeForStat(%q) = %q, want %q", statID, got, want)
		}
	}
}
