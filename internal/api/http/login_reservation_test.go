package http

import (
	"net/url"
	"os"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/database"
	"taunewlety/pkg"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// In-flight credential checks count against the per-IP limit, so concurrent
// requests cannot all be admitted before any failure is recorded.
func TestLoginRateLimiter_ReserveCountsInFlight(t *testing.T) {
	l := &loginRateLimiter{attempts: map[string]*loginAttempt{}, maxAttempts: 3, window: time.Minute}

	for i := 0; i < 3; i++ {
		if !l.reserve("1.1.1.1") {
			t.Fatalf("reservation %d should be admitted", i+1)
		}
	}
	if l.reserve("1.1.1.1") {
		t.Fatal("a fourth concurrent reservation must be rejected")
	}
	if l.check("1.1.1.1") {
		t.Error("check must also count in-flight reservations")
	}

	l.release("1.1.1.1", false)
	if !l.reserve("1.1.1.1") {
		t.Error("a released non-failure should free capacity")
	}

	for i := 0; i < 3; i++ {
		l.release("1.1.1.1", true)
	}
	if l.reserve("1.1.1.1") {
		t.Error("three recorded failures should exhaust the limit")
	}
}

func TestLoginRateLimiter_ReleaseAfterReset(t *testing.T) {
	l := &loginRateLimiter{attempts: map[string]*loginAttempt{}, maxAttempts: 3, window: time.Minute}
	l.reserve("1.1.1.1")
	l.reserve("1.1.1.1")
	l.reset("1.1.1.1") // a concurrent request logged in successfully
	l.release("1.1.1.1", true)
	if a := l.attempts["1.1.1.1"]; a == nil || a.count != 1 || a.inFlight != 0 {
		t.Errorf("failure after reset should still be counted, got %+v", a)
	}
}

// Once the threshold is reached the account backoff admits only one check
// at a time, so a burst cannot slip past before the first failure lands.
func TestAccountBackoff_ReserveSerializesPastThreshold(t *testing.T) {
	b := &accountBackoff{threshold: 2, maxDelay: time.Millisecond, window: time.Minute}

	for i := 0; i < 2; i++ {
		if !b.reserve() {
			t.Fatalf("reservation %d below the threshold should be admitted", i+1)
		}
	}
	if b.reserve() {
		t.Fatal("in-flight checks must count toward the threshold")
	}
	b.release(true)
	b.release(true)
	time.Sleep(5 * time.Millisecond) // let the backoff delay elapse

	if !b.reserve() {
		t.Fatal("one check should be admitted once the delay has passed")
	}
	if b.reserve() {
		t.Error("past the threshold only one check may be in flight")
	}
	b.release(false)
	if b.inFlight != 0 {
		t.Errorf("expected no in-flight checks, got %d", b.inFlight)
	}
}

// Differently cased mailboxes are distinct subscribers: a token for one must
// never delete the other.
func TestUnsubscribe_CaseDistinctMailboxes(t *testing.T) {
	sent := make(chan string, 4)
	originalSendEmail := sendUnsubscribeEmail
	sendUnsubscribeEmail = func(email string) { sent <- email }
	defer func() { sendUnsubscribeEmail = originalSendEmail }()

	_ = os.Setenv("DB_PATH", ":memory:")
	defer func() { _ = os.Unsetenv("DB_PATH") }()
	db, err := database.InitDB()
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	t.Setenv("SESSION_SECRET", unsubTestSecret)
	t.Setenv("UNSUBSCRIBE_SECRET", "")

	db.Create(&models.Subscriber{Email: "Bob@Example.com"})
	db.Create(&models.Subscriber{Email: "bob@example.com"})

	gin.SetMode(gin.TestMode)
	r, err := SetupRouter(db, zap.NewNop())
	if err != nil {
		t.Fatalf("SetupRouter failed: %v", err)
	}

	if pkg.UnsubscribeToken(unsubTestSecret, "bob@example.com") == pkg.UnsubscribeToken(unsubTestSecret, "Bob@example.com") {
		t.Fatal("case-distinct local parts must not share a token")
	}

	// The lowercase mailbox's link, submitted with the other spelling.
	cookieVal, csrfToken := openUnsubscribeForm(t, r, "bob@example.com")
	form := url.Values{}
	form.Set("csrf_token", csrfToken)
	form.Set("email", "Bob@example.com")
	form.Set("token", pkg.UnsubscribeToken(unsubTestSecret, "bob@example.com"))
	if w := postUnsubscribe(r, cookieVal, form); w.Code != 403 {
		t.Fatalf("token for bob@ must not authorize Bob@, got %d", w.Code)
	}

	// The legitimate request (domain case differs, which is insignificant)
	// removes only the lowercase mailbox.
	cookieVal, csrfToken = openUnsubscribeForm(t, r, "bob@EXAMPLE.com")
	form = url.Values{}
	form.Set("csrf_token", csrfToken)
	form.Set("email", "bob@EXAMPLE.com")
	form.Set("token", pkg.UnsubscribeToken(unsubTestSecret, "bob@EXAMPLE.com"))
	if w := postUnsubscribe(r, cookieVal, form); w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := <-sent; got != "bob@example.com" {
		t.Errorf("confirmation should go to the removed mailbox, got %q", got)
	}

	var remaining []models.Subscriber
	db.Unscoped().Find(&remaining)
	if len(remaining) != 1 || remaining[0].Email != "Bob@Example.com" {
		t.Errorf("only bob@example.com should be removed, remaining: %+v", remaining)
	}
}
