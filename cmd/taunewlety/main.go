package main

import (
	"context"
	"log"
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
	app := taunewlety.NewApp()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setupSignalHandler(cancel)

	_ = app.Run(ctx)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	onShutdown(app.Shutdown(shutdownCtx))
}
