package http

import (
	"bufio"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"taunewlety/internal/platform/clients"
	"taunewlety/internal/platform/database"
	"testing"

	"github.com/gin-gonic/gin"
)

type mockSMTPServer struct {
	listener net.Listener
}

func startMockSMTPServer(t *testing.T) *mockSMTPServer {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock SMTP server: %v", err)
	}

	server := &mockSMTPServer{listener: l}

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				reader := bufio.NewReader(c)
				writer := bufio.NewWriter(c)

				_, _ = writer.WriteString("220 mock-smtp ready\r\n")
				_ = writer.Flush()

				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						break
					}
					cmd := strings.ToUpper(strings.TrimSpace(line))
					if strings.HasPrefix(cmd, "EHLO") || strings.HasPrefix(cmd, "HELO") {
						_, _ = writer.WriteString("250-localhost\r\n250 AUTH PLAIN\r\n")
					} else if strings.HasPrefix(cmd, "AUTH PLAIN") {
						_, _ = writer.WriteString("235 Authentication successful\r\n")
					} else if strings.HasPrefix(cmd, "MAIL FROM") {
						_, _ = writer.WriteString("250 OK\r\n")
					} else if strings.HasPrefix(cmd, "RCPT TO") {
						_, _ = writer.WriteString("250 OK\r\n")
					} else if cmd == "DATA" {
						_, _ = writer.WriteString("354 Start mail input\r\n")
						_ = writer.Flush()
						for {
							bodyLine, err := reader.ReadString('\n')
							if err != nil || strings.TrimSpace(bodyLine) == "." {
								break
							}
						}
						_, _ = writer.WriteString("250 OK\r\n")
					} else if cmd == "QUIT" {
						_, _ = writer.WriteString("221 Bye\r\n")
						_ = writer.Flush()
						break
					} else {
						log.Printf("MOCK SMTP RECEIVED: %q", line)
						_, _ = writer.WriteString("500 Unknown command\r\n")
					}
					_ = writer.Flush()
				}
			}(conn)
		}
	}()

	return server
}

func (s *mockSMTPServer) Close() {
	if s.listener != nil {
		_ = s.listener.Close()
	}
}

func TestNewsletterHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("NOTIFY_EMAIL", "notify@example.com")
	defer os.Unsetenv("NOTIFY_EMAIL")

	t.Run("NewsletterPreview", func(t *testing.T) {
		tests := []struct {
			name           string
			setupDB        func()
			setupServer    func(w http.ResponseWriter, r *http.Request)
			expectedStatus int
			expectedBody   string
		}{
			{
				name: "ConfigNil",
				setupDB: func() {
					database.InitDB(":memory:")
					database.DB.Exec("DELETE FROM configs")
				},
				expectedStatus: http.StatusBadRequest,
				expectedBody:   "Configure settings first",
			},
			{
				name: "GenerateError",
				setupDB: func() {
					database.InitDB(":memory:")
				},
				setupServer: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				},
				expectedStatus: http.StatusInternalServerError,
			},
			{
				name: "Success",
				setupDB: func() {
					database.InitDB(":memory:")
				},
				setupServer: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if strings.Contains(r.URL.Path, "generate") {
						_ = json.NewEncoder(w).Encode(clients.OllamaResponse{Response: `{"subject": "test", "body": "hello"}`})
					} else {
						payload := map[string]interface{}{
							"response": map[string]interface{}{
								"data": map[string]interface{}{
									"recently_added": []interface{}{
										map[string]interface{}{"rating_key": "1", "title": "Test"},
									},
								},
							},
						}
						_ = json.NewEncoder(w).Encode(payload)
					}
				},
				expectedStatus: http.StatusOK,
				expectedBody:   "hello",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tt.setupDB()
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if tt.setupServer != nil {
						tt.setupServer(w, r)
					}
				}))
				defer ts.Close()

				if config, _ := database.GetConfig(); config != nil {
					config.TautulliURL = ts.URL
					config.OllamaURL = ts.URL
					_ = database.SaveConfig(config)
				}

				h := NewHandler()
				r := gin.New()
				r.LoadHTMLGlob(filepath.Join(resolveWebDir(), "template", "*"))
				r.GET("/preview", h.NewsletterPreview)

				w := httptest.NewRecorder()
				req, _ := http.NewRequest(http.MethodGet, "/preview", nil)
				r.ServeHTTP(w, req)

				if w.Code != tt.expectedStatus {
					t.Errorf("expected %d, got %d", tt.expectedStatus, w.Code)
				}
				if tt.expectedBody != "" && !strings.Contains(w.Body.String(), tt.expectedBody) {
					t.Errorf("expected body to contain %q, got %q", tt.expectedBody, w.Body.String())
				}
			})
		}
	})

	t.Run("NewsletterSendManual", func(t *testing.T) {
		tests := []struct {
			name           string
			setupDB        func()
			setupServer    func(w http.ResponseWriter, r *http.Request)
			smtpFail       bool
			expectedStatus int
			expectedBody   string
		}{
			{
				name: "ConfigNil",
				setupDB: func() {
					database.InitDB(":memory:")
					database.DB.Exec("DELETE FROM configs")
				},
				expectedStatus: http.StatusBadRequest,
				expectedBody:   "Configure settings first",
			},
			{
				name: "NoRecommendations",
				setupDB: func() {
					database.InitDB(":memory:")
				},
				setupServer: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					payload := map[string]interface{}{
						"response": map[string]interface{}{
							"data": map[string]interface{}{
								"recently_added": []interface{}{},
							},
						},
					}
					_ = json.NewEncoder(w).Encode(payload)
				},
				expectedStatus: http.StatusOK,
				expectedBody:   "Skipped",
			},
			{
				name: "GenerateError",
				setupDB: func() {
					database.InitDB(":memory:")
				},
				setupServer: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				},
				expectedStatus: http.StatusInternalServerError,
			},
			{
				name: "SMTPSendError",
				setupDB: func() {
					database.InitDB(":memory:")
				},
				setupServer: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if strings.Contains(r.URL.Path, "generate") {
						_ = json.NewEncoder(w).Encode(clients.OllamaResponse{Response: `{"subject": "test", "body": "hello"}`})
					} else {
						payload := map[string]interface{}{
							"response": map[string]interface{}{
								"data": map[string]interface{}{
									"recently_added": []interface{}{
										map[string]interface{}{"rating_key": "1", "title": "Test"},
									},
								},
							},
						}
						_ = json.NewEncoder(w).Encode(payload)
					}
				},
				smtpFail:       true,
				expectedStatus: http.StatusInternalServerError,
			},
			{
				name: "Success",
				setupDB: func() {
					database.InitDB(":memory:")
				},
				setupServer: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if strings.Contains(r.URL.Path, "generate") {
						_ = json.NewEncoder(w).Encode(clients.OllamaResponse{Response: `{"subject": "test", "body": "hello"}`})
					} else {
						payload := map[string]interface{}{
							"response": map[string]interface{}{
								"data": map[string]interface{}{
									"recently_added": []interface{}{
										map[string]interface{}{"rating_key": "1", "title": "Test"},
									},
								},
							},
						}
						_ = json.NewEncoder(w).Encode(payload)
					}
				},
				expectedStatus: http.StatusOK,
				expectedBody:   "Sent",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tt.setupDB()
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if tt.setupServer != nil {
						tt.setupServer(w, r)
					}
				}))
				defer ts.Close()

				var smtpSrv *mockSMTPServer
				if !tt.smtpFail {
					smtpSrv = startMockSMTPServer(t)
					defer smtpSrv.Close()
				}

				if config, _ := database.GetConfig(); config != nil {
					config.TautulliURL = ts.URL
					config.OllamaURL = ts.URL
					if smtpSrv != nil {
						addr := smtpSrv.listener.Addr().(*net.TCPAddr)
						config.SMTPHost = addr.IP.String()
						config.SMTPPort = addr.Port
					} else {
						// Obtain a free port then close it so the connection is
						// refused deterministically (avoids a flaky hardcoded port).
						l, err := net.Listen("tcp", "127.0.0.1:0")
						if err != nil {
							t.Fatalf("failed to reserve port: %v", err)
						}
						refusedPort := l.Addr().(*net.TCPAddr).Port
						_ = l.Close()
						config.SMTPHost = "127.0.0.1"
						config.SMTPPort = refusedPort
					}
					_ = database.SaveConfig(config)
				}

				h := NewHandler()
				r := gin.New()
				r.POST("/send", h.NewsletterSendManual)

				w := httptest.NewRecorder()
				req, _ := http.NewRequest(http.MethodPost, "/send", nil)
				r.ServeHTTP(w, req)

				if w.Code != tt.expectedStatus {
					t.Errorf("expected %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
				}
				if tt.expectedBody != "" && !strings.Contains(w.Body.String(), tt.expectedBody) {
					t.Errorf("expected body to contain %q, got %q", tt.expectedBody, w.Body.String())
				}
			})
		}
	})

	t.Run("SendManual_GetConfigError", func(t *testing.T) {
		database.InitDB(":memory:")
		sqlDB, _ := database.DB.DB()
		sqlDB.Close()

		h := NewHandler()
		r := gin.New()
		r.POST("/send", h.NewsletterSendManual)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/send", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected BadRequest (400), got: %d", w.Code)
		}
	})
}
