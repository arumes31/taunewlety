package main

import (
	"log"
	"taunewlety/internal/app/taunewlety"
)

func main() {
	app := taunewlety.NewApp()
	if err := app.Run(); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}
