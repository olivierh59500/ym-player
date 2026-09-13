package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/olivierh59500/ym-player/pkg/audio"
	"github.com/olivierh59500/ym-player/pkg/stsound"
)

var (
	sampleRate = flag.Int("rate", 44100, "Sample rate (Hz)")
	bufferSize = flag.Int("buffer", 2048, "Buffer size")
	loop       = flag.Bool("loop", false, "Loop playback")
	volume     = flag.Float64("volume", 1.0, "Volume (0.0 to 10.0)")
	gain       = flag.Float64("gain", 1.0, "Audio gain multiplier")
	lowpass    = flag.Bool("lowpass", true, "Enable lowpass filter")
	info       = flag.Bool("info", false, "Show file info only")
	output     = flag.String("output", "oto", "Output backend (oto, wav, null)")
	wavFile    = flag.String("wav", "", "Output WAV file (when using wav output)")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <ym-file>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "YM Player - Play Atari ST YM music files\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	ymFile := flag.Arg(0)

	// Check if file exists
	if _, err := os.Stat(ymFile); os.IsNotExist(err) {
		log.Fatalf("File not found: %s", ymFile)
	}

	// Try to get file info first
	data, err := os.ReadFile(ymFile)
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}

	format, compressed, err := stsound.GetYMInfo(data)
	if err != nil {
		log.Fatalf("Failed to identify file format: %v", err)
	}

	fmt.Printf("File format: %s", format)
	if compressed {
		fmt.Printf(" (compressed)")
	}
	fmt.Printf("\n")

	// Create YM player
	player := stsound.CreateWithRate(*sampleRate)
	defer player.Destroy()

	// Load YM file
	fmt.Printf("Loading %s...\n", filepath.Base(ymFile))
	if err := player.LoadMemory(data); err != nil {
		log.Fatalf("Failed to load YM file: %v", err)
	}

	// Get and display info
	musicInfo := player.GetInfo()
	fmt.Printf("\n")
	fmt.Printf("Title:    %s\n", musicInfo.SongName)
	fmt.Printf("Author:   %s\n", musicInfo.SongAuthor)
	fmt.Printf("Comment:  %s\n", musicInfo.SongComment)
	fmt.Printf("Type:     %s\n", musicInfo.SongType)
	fmt.Printf("Duration: %s\n", formatDuration(uint32(musicInfo.MusicTimeInMs)))
	fmt.Printf("\n")

	if *info {
		// Info only mode
		return
	}

	// Set options
	player.SetLoopMode(*loop)
	player.SetLowpassFilter(*lowpass)

	// Create audio output
	var audioOut audio.Output

	switch *output {
	case "oto":
		audioOut, err = audio.NewStreamingOtoOutput()
		if err != nil {
			fmt.Printf("Warning: Failed to create audio output (%v)\n", err)
			fmt.Printf("Falling back to timing-based output...\n")
			audioOut, err = audio.NewFallbackOutput()
		}
	case "wav":
		if *wavFile == "" {
			*wavFile = strings.TrimSuffix(ymFile, filepath.Ext(ymFile)) + ".wav"
		}
		audioOut, err = createWAVOutput(*wavFile)
	case "null":
		audioOut = &NullOutput{}
		err = nil
	default:
		log.Fatalf("Unknown output backend: %s", *output)
	}

	if err != nil {
		log.Fatalf("Failed to create audio output: %v", err)
	}

	// Open audio output
	if err := audioOut.Open(*sampleRate, 1, *bufferSize); err != nil {
		log.Fatalf("Failed to open audio output: %v", err)
	}
	defer audioOut.Close()

	// Start playback
	fmt.Printf("Playing... (Press Ctrl+C to stop)\n")
	if *loop {
		fmt.Printf("Looping enabled\n")
	}
	fmt.Printf("\n")

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create done channel
	done := make(chan bool)

	// Start playback goroutine
	go func() {
		buffer := make([]int16, *bufferSize)

		player.Play()

		for {
			// Generate audio
			if !player.Compute(buffer, len(buffer)) {
				if !*loop {
					done <- true
					return
				}
			}

			// Apply volume and gain
			totalGain := *volume * *gain
			if totalGain != 1.0 {
				for i := range buffer {
					// Appliquer le gain avec saturation
					sample := float64(buffer[i]) * totalGain
					if sample > 32767 {
						buffer[i] = 32767
					} else if sample < -32768 {
						buffer[i] = -32768
					} else {
						buffer[i] = int16(sample)
					}
				}
			}

			// Write to output
			if err := audioOut.Write(buffer); err != nil {
				log.Printf("Audio write error: %v", err)
			}
		}
	}()

	// Progress display
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			fmt.Printf("\n\nStopping...\n")
			return

		case <-done:
			fmt.Printf("\n\nPlayback finished.\n")
			return

		case <-ticker.C:
			// Update progress
			pos := player.GetPos()
			total := musicInfo.MusicTimeInMs

			if total > 0 {
				percent := float64(pos) / float64(total) * 100
				fmt.Printf("\r[%s] %s / %s (%.1f%%)",
					makeProgressBar(percent, 30),
					formatDuration(uint32(pos)),
					formatDuration(uint32(total)),
					percent)
			}
		}
	}
}

func createWAVOutput(filename string) (audio.Output, error) {
	return audio.NewWAVOutput(filename), nil
}

func formatDuration(ms uint32) string {
	seconds := ms / 1000
	minutes := seconds / 60
	seconds %= 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func makeProgressBar(percent float64, width int) string {
	filled := int(percent / 100 * float64(width))
	if filled > width {
		filled = width
	}

	bar := strings.Repeat("=", filled)
	if filled < width {
		bar += ">"
		bar += strings.Repeat(" ", width-filled-1)
	}

	return bar
}

// NullOutput discards all audio while preserving real-time pacing.
type NullOutput struct {
	sampleRate int
	channels   int
}

func (n *NullOutput) Open(sampleRate, channels, bufferSize int) error {
	n.sampleRate = sampleRate
	n.channels = channels
	return nil
}

func (n *NullOutput) Close() error {
	return nil
}

func (n *NullOutput) Write(samples []int16) error {
	// Simulate write delay
	duration := time.Duration(len(samples)) * time.Second / time.Duration(n.sampleRate*n.channels)
	time.Sleep(duration)
	return nil
}

func (n *NullOutput) IsPlaying() bool {
	return true
}
