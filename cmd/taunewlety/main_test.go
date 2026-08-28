package main

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestHealthcheckURL(t *testing.T) {
	t.Setenv("PORT", "9090")
	if got := healthcheckURL(); got != "http://127.0.0.1:9090/health" {
		t.Fatalf("healthcheckURL() = %q", got)
	}
	t.Setenv("TLS_CERT", "/cert.pem")
	t.Setenv("TLS_KEY", "/key.pem")
	if got := healthcheckURL(); got != "https://127.0.0.1:9090/health" {
		t.Fatalf("TLS healthcheckURL() = %q", got)
	}
}

// freePort reserves an ephemeral port and releases it, returning the port
// number as a string. This avoids flaky failures from hardcoded ports that
// may already be in use on the test machine.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a free port: %v", err)
	}
	defer func() { _ = l.Close() }()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

func TestMainFunc(t *testing.T) {
	_ = os.Setenv("PORT", freePort(t))
	_ = os.Setenv("DB_PATH", ":memory:")
	_ = os.Setenv("SESSION_SECRET", "main-test-secret-9876")
	_ = os.Setenv("APP_USER", "admin")
	_ = os.Setenv("APP_PASS", "password")
	defer func() {
		_ = os.Unsetenv("PORT")
		_ = os.Unsetenv("DB_PATH")
		_ = os.Unsetenv("SESSION_SECRET")
		_ = os.Unsetenv("APP_USER")
		_ = os.Unsetenv("APP_PASS")
	}()

	var triggerCancel context.CancelFunc

	oldSetupSignalHandler := setupSignalHandler
	defer func() { setupSignalHandler = oldSetupSignalHandler }()

	setupSignalHandler = func(cancel context.CancelFunc) {
		triggerCancel = cancel
	}

	mainChan := make(chan struct{})
	go func() {
		main()
		close(mainChan)
	}()

	// Give the app some time to start up and listen
	time.Sleep(300 * time.Millisecond)

	if triggerCancel == nil {
		t.Fatal("expected setupSignalHandler to set triggerCancel, but it is nil")
	}

	// Trigger cancel
	triggerCancel()

	// Wait for main to exit
	select {
	case <-mainChan:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for main() to exit")
	}
}

func TestDefaultSetupSignalHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use original setupSignalHandler
	setupSignalHandler(cancel)

	// Simulate signal
	sigChan <- syscall.SIGINT

	// Verify context is cancelled
	select {
	case <-ctx.Done():
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("expected context to be cancelled, but it was not")
	}
}

func TestMainFunc_ShutdownError(t *testing.T) {
	port := freePort(t)
	_ = os.Setenv("PORT", port)
	_ = os.Setenv("DB_PATH", ":memory:")
	_ = os.Setenv("SESSION_SECRET", "main-test-secret-9876")
	_ = os.Setenv("APP_USER", "admin")
	_ = os.Setenv("APP_PASS", "password")
	defer func() {
		_ = os.Unsetenv("PORT")
		_ = os.Unsetenv("DB_PATH")
		_ = os.Unsetenv("SESSION_SECRET")
		_ = os.Unsetenv("APP_USER")
		_ = os.Unsetenv("APP_PASS")
	}()

	// Force Shutdown to time out/error immediately
	oldTimeout := shutdownTimeout
	shutdownTimeout = 1 * time.Nanosecond
	defer func() { shutdownTimeout = oldTimeout }()

	// Capture the shutdown result so we can assert the timeout was hit.
	shutdownErrChan := make(chan error, 1)
	oldOnShutdown := onShutdown
	onShutdown = func(err error) { shutdownErrChan <- err }
	defer func() { onShutdown = oldOnShutdown }()

	var triggerCancel context.CancelFunc
	oldSetupSignalHandler := setupSignalHandler
	defer func() { setupSignalHandler = oldSetupSignalHandler }()

	setupSignalHandler = func(cancel context.CancelFunc) {
		triggerCancel = cancel
	}

	mainChan := make(chan struct{})
	go func() {
		main()
		close(mainChan)
	}()

	// Wait for server to start listening
	var connected bool
	var activeConn net.Conn
	for i := 0; i < 100; i++ {
		conn, err := net.Dial("tcp", "127.0.0.1:"+port)
		if err == nil {
			activeConn = conn
			connected = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !connected {
		t.Fatal("timed out waiting for server to start listening")
	}
	// Hold the connection open so the (1ns) shutdown context deadline is
	// exceeded before the server can drain, forcing a shutdown error.
	defer func() { _ = activeConn.Close() }()

	if triggerCancel == nil {
		t.Fatal("expected setupSignalHandler to set triggerCancel, but it is nil")
	}

	triggerCancel()

	select {
	case <-mainChan:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for main() to exit")
	}

	// The forced 1ns timeout with an open connection must surface as an error.
	select {
	case err := <-shutdownErrChan:
		if err == nil {
			t.Fatal("expected a non-nil shutdown error from the forced timeout, got nil")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context.DeadlineExceeded, got: %v", err)
		}
	default:
		t.Fatal("expected onShutdown to be invoked with the shutdown result")
	}
}
