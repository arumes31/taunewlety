package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"taunewlety/internal/app/taunewlety"
	"time"
)

var sigChan = make(chan os.Signal, 1)
var shutdownTimeout = 10 * time.Second

// onShutdown is invoked with the result of app.Shutdown. It is a package
// variable so tests can observe the shutdown outcome (e.g. a timeout error).
var onShutdown = func(err error) {
	if err != nil {
		log.Printf("Application shutdown error: %v", err)
	}
}

var setupSignalHandler = func(cancel context.CancelFunc) {
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(); err != nil {
			log.Printf("Healthcheck failed: %v", err)
			os.Exit(1)
		}
		return
	}

	app := taunewlety.NewApp()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setupSignalHandler(cancel)

	_ = app.Run(ctx)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	onShutdown(app.Shutdown(shutdownCtx))
}

func runHealthcheck() error {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if os.Getenv("TLS_CERT") != "" && os.Getenv("TLS_KEY") != "" {
		// The probe is strictly loopback-only and validates application liveness,
		// while the public TLS endpoint remains fully verified by clients.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402
	}
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}

	response, err := client.Get(healthcheckURL())
	if err != nil {
		return fmt.Errorf("request health endpoint: %w", err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			log.Printf("Close healthcheck response: %v", err)
		}
	}()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned %s", response.Status)
	}
	return nil
}

func healthcheckURL() string {
	scheme := "http"
	if os.Getenv("TLS_CERT") != "" && os.Getenv("TLS_KEY") != "" {
		scheme = "https"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return scheme + "://127.0.0.1:" + port + "/health"
}
