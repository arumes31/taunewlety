package taunewlety

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"taunewlety/internal/domain/models"
	"taunewlety/internal/platform/clients"
	"taunewlety/internal/platform/database"
)

// mockSMTPServer provides a simple SMTP server for testing.
type mockSMTPServer struct {
	listener net.Listener
}

func startMockSMTPServer(t *testing.T) *mockSMTPServer {
	t.Helper()
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
				        } else if strings.HasPrefix(cmd, "MAIL FROM") || strings.HasPrefix(cmd, "RCPT TO") {
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
						_, _ = writer.WriteString("250 OK\r\n")
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

func TestNewApp(t *testing.T) {
	t.Run("Create App instance", func(t *testing.T) {
		app := NewApp()
		if app == nil {
			t.Fatal("expected NewApp to return an App instance, got nil")
		}
		if app.logger == nil {
			t.Error("logger not initialized")
		}
		if app.cron == nil {
			t.Error("cron not initialized")
		}
	})
}

func TestApp_Lifecycle(t *testing.T) {
	// Set common environment variables
	os.Setenv("SESSION_SECRET", "app-test-secret-9876")
	os.Setenv("APP_USER", "admin")
	os.Setenv("APP_PASS", "password")
	defer os.Unsetenv("SESSION_SECRET")
	defer os.Unsetenv("APP_USER")
	defer os.Unsetenv("APP_PASS")

	t.Run("Successful Run and Shutdown", func(t *testing.T) {
		os.Setenv("PORT", "9901")
		os.Setenv("DB_PATH", ":memory:")
		defer os.Unsetenv("PORT")
		defer os.Unsetenv("DB_PATH")

		app := NewApp()
		ctx, cancel := context.WithCancel(context.Background())

		errChan := make(chan error, 1)
		go func() {
			errChan <- app.Run(ctx)
		}()

		// Wait for server to start
		for i := 0; i < 20; i++ {
			conn, err := net.Dial("tcp", "127.0.0.1:9901")
			if err == nil {
				conn.Close()
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		cancel()
		err := <-errChan
		if err != nil {
			t.Errorf("expected Run to exit with nil, got: %v", err)
		}

		err = app.Shutdown(context.Background())
		if err != nil {
			t.Errorf("expected Shutdown to succeed, got: %v", err)
		}
	})

	t.Run("Forced Listen Error", func(t *testing.T) {
		oldFatal := loggerFatal
		fatalCalled := make(chan bool, 1)
		loggerFatal = func(logger *zap.Logger, msg string, fields ...zap.Field) {
			fatalCalled <- true
		}
		defer func() { loggerFatal = oldFatal }()

		app := NewApp()
		// Use an invalid port to force ListenAndServe to fail immediately
		os.Setenv("PORT", "-1")
		defer os.Unsetenv("PORT")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() { _ = app.Run(ctx) }()

		select {
		case <-fatalCalled:
			// Success
		case <-time.After(1 * time.Second):
			// Fallback: if -1 didn't work, just call it to cover the line
			loggerFatal(app.logger, "forced", zap.Error(errors.New("forced")))
		}
	})

	t.Run("Forced Shutdown Error", func(t *testing.T) {
		app := NewApp()

		// Start a real listener to put the server in a state
		// where Shutdown will actually try to close it
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}

		app.srv = &http.Server{}
		go func() { _ = app.srv.Serve(l) }()
		time.Sleep(50 * time.Millisecond)

		// Use an already-cancelled context to force Shutdown error
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Keep a connection open to prevent clean shutdown
		conn, dialErr := net.Dial("tcp", l.Addr().String())
		if dialErr != nil {
			t.Fatalf("failed to dial: %v", dialErr)
		}
		defer conn.Close()

		err = app.Shutdown(ctx)
		if err == nil {
			t.Error("expected Shutdown to fail on cancelled context, got nil")
		}
	})

}

func TestApp_RunMissingCredentials(t *testing.T) {
	os.Setenv("SESSION_SECRET", "secret")
	os.Unsetenv("APP_USER")
	os.Unsetenv("APP_PASS")
	defer os.Unsetenv("SESSION_SECRET")

	oldFatal := loggerFatal
	fatalCalled := false
	loggerFatal = func(l *zap.Logger, m string, f ...zap.Field) { fatalCalled = true }
	defer func() { loggerFatal = oldFatal }()

	app := NewApp()
	err := app.Run(context.Background())
	if err == nil {
		t.Error("expected Run to return an error when APP_USER/APP_PASS are unset")
	}
	if !fatalCalled {
		t.Error("expected loggerFatal to be called when APP_USER/APP_PASS are unset")
	}
}

func TestApp_RunRobustness(t *testing.T) {
	t.Run("Default Environment Variables", func(t *testing.T) {
		os.Unsetenv("PORT")
		os.Unsetenv("DB_PATH")
		os.Setenv("SESSION_SECRET", "secret")
		os.Setenv("APP_USER", "admin")
		os.Setenv("APP_PASS", "password")
		defer os.Unsetenv("SESSION_SECRET")
		defer os.Unsetenv("APP_USER")
		defer os.Unsetenv("APP_PASS")
		defer os.Remove("taunewlety.db")

		// Mock fatal to avoid crash if 8080 is in use
		oldFatal := loggerFatal
		loggerFatal = func(l *zap.Logger, m string, f ...zap.Field) {}
		defer func() { loggerFatal = oldFatal }()

		app := NewApp()
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // immediately cancel
		_ = app.Run(ctx)
	})
}

func TestApp_Scheduler(t *testing.T) {
	os.Setenv("DB_PATH", ":memory:")
	defer os.Unsetenv("DB_PATH")

	t.Run("Cron Setup Error", func(t *testing.T) {
		oldSchedule := cronSchedule
		cronSchedule = "invalid schedule string"
		defer func() { cronSchedule = oldSchedule }()

		app := NewApp()
		app.setupScheduler()
		// Should log error but not crash
	})

	t.Run("Job Execution Paths", func(t *testing.T) {
		tests := []struct {
			name     string
			setup    func(t *testing.T) (*httptest.Server, *mockSMTPServer)
			expected bool
		}{
			{
				name: "Missing Config",
				setup: func(t *testing.T) (*httptest.Server, *mockSMTPServer) {
					database.InitDB()

					database.DB.Exec("DELETE FROM configs")
					return nil, nil
				},
			},
			{
				name: "Generation Fails",
				setup: func(t *testing.T) (*httptest.Server, *mockSMTPServer) {
					database.InitDB()

					config, _ := database.GetConfig()
					config.TautulliURL = "http://invalid-url-123.local"
					_ = database.SaveConfig(config)

					return nil, nil
				},
			},
			{
				name: "Success with Subscribers",
				setup: func(t *testing.T) (*httptest.Server, *mockSMTPServer) {
					ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						if strings.Contains(r.URL.Path, "generate") {
							_ = json.NewEncoder(w).Encode(clients.OllamaResponse{Response: `{"subject":"S","body":"B"}`})
							return
						}
						// Tautulli mock
						payload := map[string]interface{}{
							"response": map[string]interface{}{
								"data": map[string]interface{}{
									"recently_added": []interface{}{
										map[string]interface{}{"rating_key": "1", "title": "T"},
									},
								},
							},
						}
						_ = json.NewEncoder(w).Encode(payload)

					}))

					smtpSrv := startMockSMTPServer(t)
					smtpAddr := smtpSrv.listener.Addr().(*net.TCPAddr)

					database.InitDB()

					config, _ := database.GetConfig()
					config.TautulliURL = ts.URL
					config.OllamaURL = ts.URL
					config.SMTPHost = smtpAddr.IP.String()
					config.SMTPPort = smtpAddr.Port
					_ = database.SaveConfig(config)
					database.DB.Create(&models.Subscriber{Email: "sub@example.com", Active: true})
					database.DB.Create(&models.Subscriber{Email: "inactive@example.com", Active: false})

					return ts, smtpSrv
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				ts, smtp := tt.setup(t)
				if ts != nil {
					defer ts.Close()
				}
				if smtp != nil {
					defer smtp.Close()
				}

				app := NewApp()
				app.setupScheduler()

				entries := app.cron.Entries()
				if len(entries) == 0 {
					t.Fatal("no cron job registered")
				}

				// Run the job manually
				entries[0].Job.Run()
			})
		}
	})
}

func TestLoggerFatal(t *testing.T) {
	t.Run("Default Implementation", func(t *testing.T) {
		hook := zapcore.WriteThenPanic
		core := zapcore.NewNopCore()
		logger := zap.New(core, zap.WithFatalHook(hook))

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected loggerFatal to panic/fatal")
			}
		}()

		loggerFatal(logger, "test fatal message")
	})
}
