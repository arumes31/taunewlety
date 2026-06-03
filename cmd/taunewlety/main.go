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

func main() {
	app := taunewlety.NewApp()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	if err := app.Run(ctx); err != nil {
		log.Printf("Application run error: %v", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := app.Shutdown(shutdownCtx); err != nil {
		log.Printf("Application shutdown error: %v", err)
	}
}
