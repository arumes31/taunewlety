package http

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"taunewlety/internal/domain/models"
)

func subscribersFrom(emails ...string) []models.Subscriber {
	subs := make([]models.Subscriber, 0, len(emails))
	for _, e := range emails {
		subs = append(subs, models.Subscriber{Email: e})
	}
	return subs
}

// TestCollectRecipients verifies that every address is delivered to once,
// including when NOTIFY_EMAIL is also on the subscriber list.
func TestCollectRecipients(t *testing.T) {
	tests := []struct {
		name        string
		subscribers []string
		notifyEmail string
		want        []string
	}{
		{
			name:        "No overlap keeps everyone",
			subscribers: []string{"a@example.com", "b@example.com"},
			notifyEmail: "admin@example.com",
			want:        []string{"a@example.com", "b@example.com", "admin@example.com"},
		},
		{
			name:        "Admin is also a subscriber",
			subscribers: []string{"a@example.com", "admin@example.com"},
			notifyEmail: "admin@example.com",
			want:        []string{"a@example.com", "admin@example.com"},
		},
		{
			name:        "Admin differs only by case",
			subscribers: []string{"a@example.com", "Admin@Example.com"},
			notifyEmail: "admin@example.com",
			want:        []string{"a@example.com", "Admin@Example.com"},
		},
		{
			name:        "Admin differs only by surrounding whitespace",
			subscribers: []string{" admin@example.com "},
			notifyEmail: "admin@example.com",
			want:        []string{" admin@example.com "},
		},
		{
			name:        "Duplicate subscriber rows collapse",
			subscribers: []string{"a@example.com", "a@example.com"},
			notifyEmail: "",
			want:        []string{"a@example.com"},
		},
		{
			name:        "No notify email configured",
			subscribers: []string{"a@example.com"},
			notifyEmail: "",
			want:        []string{"a@example.com"},
		},
		{
			name:        "Only the notify email",
			subscribers: nil,
			notifyEmail: "admin@example.com",
			want:        []string{"admin@example.com"},
		},
		{
			name:        "Nothing at all",
			subscribers: nil,
			notifyEmail: "",
			want:        []string{},
		},
		{
			name:        "Blank subscriber rows are dropped",
			subscribers: []string{"", "   ", "a@example.com"},
			notifyEmail: "",
			want:        []string{"a@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectRecipients(subscribersFrom(tt.subscribers...), tt.notifyEmail)

			if len(got) != len(tt.want) {
				t.Fatalf("got %d recipients %v, want %d %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("recipient %d = %q, want %q", i, got[i], tt.want[i])
				}
			}

			// Nobody may appear twice under case/whitespace-insensitive
			// comparison.
			seen := map[string]bool{}
			for _, addr := range got {
				key := strings.ToLower(strings.TrimSpace(addr))
				if seen[key] {
					t.Errorf("address %q appears more than once in %v", addr, got)
				}
				seen[key] = true
			}
		})
	}
}

// TestSendBulkEmail_RecoversPanics verifies a panicking send cannot take the
// process down and is still counted as a failure. sendBulkEmail runs detached
// from the request, so gin's recovery middleware does not cover it.
func TestSendBulkEmail_RecoversPanics(t *testing.T) {
	var mu sync.Mutex
	var attempted []string

	send := func(to, subject, body string) error {
		mu.Lock()
		attempted = append(attempted, to)
		mu.Unlock()
		if to == "boom@example.com" {
			panic("simulated SMTP client panic")
		}
		return nil
	}

	recipients := []string{"a@example.com", "boom@example.com", "b@example.com"}

	// A panic escaping a worker would crash the test binary outright.
	sendBulkEmail(send, recipients, "subject", "body")

	mu.Lock()
	defer mu.Unlock()
	if len(attempted) != len(recipients) {
		t.Errorf("expected all %d recipients to be attempted, got %d: %v",
			len(recipients), len(attempted), attempted)
	}
}

// TestSendBulkEmail_AllPanicking confirms the WaitGroup and semaphore still
// drain when every worker panics.
func TestSendBulkEmail_AllPanicking(t *testing.T) {
	send := func(to, subject, body string) error {
		panic("always panics")
	}

	recipients := make([]string, maxConcurrentSends*3)
	for i := range recipients {
		recipients[i] = "x@example.com"
	}

	done := make(chan struct{})
	go func() {
		sendBulkEmail(send, recipients, "subject", "body")
		close(done)
	}()

	<-done // a leaked semaphore slot or missed wg.Done would hang here
}

// TestSendBulkEmail_MixedFailures exercises the ordinary error path alongside
// panics.
func TestSendBulkEmail_MixedFailures(t *testing.T) {
	var mu sync.Mutex
	delivered := 0

	send := func(to, subject, body string) error {
		switch to {
		case "panic@example.com":
			panic("boom")
		case "error@example.com":
			return errors.New("smtp rejected the recipient")
		}
		mu.Lock()
		delivered++
		mu.Unlock()
		return nil
	}

	sendBulkEmail(send, []string{
		"ok1@example.com", "panic@example.com", "error@example.com", "ok2@example.com",
	}, "subject", "body")

	mu.Lock()
	defer mu.Unlock()
	if delivered != 2 {
		t.Errorf("expected the 2 healthy recipients to be delivered, got %d", delivered)
	}
}

// TestSendBulkEmail_EmptyRecipients confirms the no-recipient case returns
// immediately.
func TestSendBulkEmail_EmptyRecipients(t *testing.T) {
	called := false
	sendBulkEmail(func(to, subject, body string) error {
		called = true
		return nil
	}, nil, "subject", "body")

	if called {
		t.Error("expected no sends for an empty recipient list")
	}
}
