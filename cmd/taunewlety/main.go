package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
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
		if err := configureHealthcheckTLS(transport, os.Getenv("TLS_CERT")); err != nil {
			return err
		}
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

func configureHealthcheckTLS(transport *http.Transport, certificatePath string) error {
	// #nosec G304,G703 -- this is the same operator-configured certificate path the server loads.
	certificatePEM, err := os.ReadFile(certificatePath)
	if err != nil {
		return fmt.Errorf("read healthcheck certificate: %w", err)
	}
	block, _ := pem.Decode(certificatePEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return errors.New("healthcheck certificate is not valid PEM")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse healthcheck certificate: %w", err)
	}
	serverName := certificate.Subject.CommonName
	if len(certificate.DNSNames) > 0 {
		serverName = certificate.DNSNames[0]
	} else if len(certificate.IPAddresses) > 0 {
		serverName = certificate.IPAddresses[0].String()
	}
	if serverName == "" {
		return errors.New("healthcheck certificate has no verifiable name")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificatePEM) {
		return errors.New("trust healthcheck certificate")
	}
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: serverName,
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
