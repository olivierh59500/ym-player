//go:build gui
// +build gui

package main

import (
	"log"
	"os"
)

func main() {
	// Disable Fyne thread checks to avoid console errors
	// This is safe because Fyne widgets are designed to be thread-safe
	os.Setenv("FYNE_DISABLETHREAD", "1")

	// Check if a file was passed as argument
	var initialFile string
	if len(os.Args) > 1 {
		initialFile = os.Args[1]
	}

	// Create and run GUI
	player := NewYMPlayerGUI()

	// Load initial file if provided
	if initialFile != "" {
		data, err := os.ReadFile(initialFile)
		if err != nil {
			log.Printf("Failed to load initial file: %v", err)
		} else {
			player.loadYMData(initialFile, data)
		}
	}

	player.Run()
}
