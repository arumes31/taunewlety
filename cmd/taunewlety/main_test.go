package main

import (
	"context"
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestMainFunc(t *testing.T) {
	os.Setenv("PORT", "9899")
	os.Setenv("DB_PATH", ":memory:")
	os.Setenv("SESSION_SECRET", "main-test-secret-9876")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("DB_PATH")
		os.Unsetenv("SESSION_SECRET")
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
	os.Setenv("PORT", "9897")
	os.Setenv("DB_PATH", ":memory:")
	os.Setenv("SESSION_SECRET", "main-test-secret-9876")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("DB_PATH")
		os.Unsetenv("SESSION_SECRET")
	}()

	// Force Shutdown to time out/error immediately
	oldTimeout := shutdownTimeout
	shutdownTimeout = 1 * time.Nanosecond
	defer func() { shutdownTimeout = oldTimeout }()

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
		conn, err := net.Dial("tcp", "127.0.0.1:9897")
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
	defer activeConn.Close()

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
}
