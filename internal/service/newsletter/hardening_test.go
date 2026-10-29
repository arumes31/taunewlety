package newsletter

import (
	"bufio"
	"context"
	"net"
	"net/smtp"
	"os"
	"strings"
	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/clients"
	"taunewlety/internal/platform/database"
	"taunewlety/pkg"
	"testing"
)

// A placeholder planted in model output must never receive the recipient's
// signed unsubscribe URL; only the application-owned footer carries it.
func TestSendEmail_DoesNotExpandPlaceholderInBody(t *testing.T) {
	original := smtpSendMail
	defer func() { smtpSendMail = original }()
	t.Setenv("UNSUBSCRIBE_SECRET", "hardening-secret")

	var sent string
	smtpSendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		sent = string(msg)
		return nil
	}

	svc := &NewsletterService{Config: &models.Config{AppBaseURL: "https://app.example", SMTPHost: "smtp.example.com", SMTPPort: 587}}
	body := `<p>Hi</p><img src="https://collector.example/x?u={{.UnsubscribeURL}}">`
	if err := svc.SendEmail("user@example.com", "S", body); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	token := pkg.UnsubscribeToken("hardening-secret", "user@example.com")
	if !strings.Contains(sent, `collector.example/x?u={{.UnsubscribeURL}}`) {
		t.Error("placeholder in body should be left untouched")
	}
	if n := strings.Count(sent, token); n != 1 {
		t.Errorf("token should appear exactly once (in the footer), got %d", n)
	}
	footerAt := strings.Index(sent, "<hr><p><small><a href=")
	if footerAt < 0 || strings.Index(sent, token) < footerAt {
		t.Error("token must only appear inside the appended footer")
	}
}

func TestAppendUnsubscribeFooter(t *testing.T) {
	got := appendUnsubscribeFooter("<html><BODY><p>x</p></BODY></html>", `https://a/u?email=a&token="t"`)
	want := `<html><BODY><p>x</p><hr><p><small><a href="https://a/u?email=a&amp;token=&#34;t&#34;">Unsubscribe</a></small></p></BODY></html>`
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	if got := appendUnsubscribeFooter("<p>x</p>", "https://a/u"); !strings.HasSuffix(got, `<a href="https://a/u">Unsubscribe</a></small></p>`) {
		t.Errorf("footer should be appended when there is no </body>: %q", got)
	}
}

// fakeSMTP serves one connection advertising the given EHLO extensions and
// reports every command it received.
func fakeSMTP(t *testing.T, ehloExt []string) (string, <-chan []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cmds := make(chan []string, 1)
	go func() {
		defer func() { _ = ln.Close() }()
		conn, err := ln.Accept()
		if err != nil {
			cmds <- nil
			return
		}
		defer func() { _ = conn.Close() }()

		var got []string
		r := bufio.NewReader(conn)
		_, _ = conn.Write([]byte("220 fake ESMTP\r\n"))
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				break
			}
			line = strings.TrimSpace(line)
			got = append(got, line)
			switch strings.ToUpper(strings.SplitN(line, " ", 2)[0]) {
			case "EHLO":
				resp := "250-fake\r\n"
				for i, e := range ehloExt {
					sep := "-"
					if i == len(ehloExt)-1 {
						sep = " "
					}
					resp += "250" + sep + e + "\r\n"
				}
				if len(ehloExt) == 0 {
					resp = "250 fake\r\n"
				}
				_, _ = conn.Write([]byte(resp))
			case "QUIT":
				_, _ = conn.Write([]byte("221 bye\r\n"))
				cmds <- got
				return
			default:
				_, _ = conn.Write([]byte("250 ok\r\n"))
			}
		}
		cmds <- got
	}()
	return ln.Addr().String(), cmds
}

// STARTTLS mode must refuse to send when the server does not offer it,
// instead of silently continuing in plain text like net/smtp.SendMail.
func TestSmtpSendMail_RefusesWithoutSTARTTLS(t *testing.T) {
	addr, cmds := fakeSMTP(t, []string{"AUTH PLAIN"})
	auth := smtp.PlainAuth("", "u", "p", "127.0.0.1")
	err := smtpSendMail(addr, auth, "from@example.com", []string{"to@example.com"}, []byte("Subject: x\r\n\r\nsecret"))
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("expected STARTTLS refusal, got %v", err)
	}
	for _, c := range <-cmds {
		switch strings.ToUpper(strings.SplitN(c, " ", 2)[0]) {
		case "AUTH", "MAIL", "RCPT", "DATA":
			t.Errorf("no credentials or envelope may be sent without TLS, got %q", c)
		}
	}
}

// A preview must not blacklist items or record stats; only a send does.
func TestPreviewNewsletter_IsReadOnly(t *testing.T) {
	_ = os.Setenv("DB_PATH", ":memory:")
	defer func() { _ = os.Unsetenv("DB_PATH") }()
	db, err := database.InitDB()
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}

	svc := &NewsletterService{
		DB:     db,
		Config: &models.Config{},
		Tautulli: &mockTautulliClient{
			GetRecentlyAddedFn: func(count int) ([]clients.RecentlyAddedItem, error) {
				return []clients.RecentlyAddedItem{{RatingKey: 4242, Title: "Preview Movie", MediaType: "movie"}}, nil
			},
		},
		Ollama: &mockOllamaClient{
			GenerateFn: func(ctx context.Context, prompt string) (string, int, int, error) {
				return `{"subject": "S", "body": "B"}`, 1, 1, nil
			},
		},
	}

	count := func(model any) int64 {
		var n int64
		db.Model(model).Where("media_id = ?", "4242").Count(&n)
		return n
	}

	if _, _, err := svc.PreviewNewsletter(context.Background()); err != nil {
		t.Fatalf("PreviewNewsletter: %v", err)
	}
	if count(&models.Blacklist{}) != 0 || count(&models.RecommendationStat{}) != 0 {
		t.Fatal("preview must not persist blacklist or recommendation stats")
	}

	if _, _, err := svc.GenerateNewsletterWithContext(context.Background()); err != nil {
		t.Fatalf("GenerateNewsletterWithContext: %v", err)
	}
	if count(&models.Blacklist{}) != 1 || count(&models.RecommendationStat{}) != 1 {
		t.Fatal("a send should record the selection")
	}
}
