package audio

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

var (
	// Global Oto context singleton
	globalOtoMutex sync.Mutex
	globalContext  *oto.Context
	globalPlayers  int
)

// StreamingOtoOutput uses Oto v3 for cross-platform audio
type StreamingOtoOutput struct {
	player     *oto.Player
	writer     *io.PipeWriter
	reader     *io.PipeReader
	sampleRate int
	channels   int
	bufferSize int
	mu         sync.Mutex
	closed     bool
	wg         sync.WaitGroup
}

// NewStreamingOtoOutput creates a new streaming Oto output
func NewStreamingOtoOutput() (*StreamingOtoOutput, error) {
	return &StreamingOtoOutput{}, nil
}

// Open opens the streaming audio output
func (s *StreamingOtoOutput) Open(sampleRate, channels, bufferSize int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.player != nil {
		return fmt.Errorf("stream already open")
	}

	s.sampleRate = sampleRate
	s.channels = channels
	s.bufferSize = bufferSize

	// Create pipe for streaming
	s.reader, s.writer = io.Pipe()

	// Get or create the global context
	globalOtoMutex.Lock()
	if globalContext == nil {
		// Create Oto context with proper buffer size for low latency
		bufferSizeInBytes := bufferSize * channels * 2 // 2 bytes per sample

		op := &oto.NewContextOptions{
			SampleRate:   sampleRate,
			ChannelCount: channels,
			Format:       oto.FormatSignedInt16LE,
			BufferSize:   time.Duration(bufferSizeInBytes) * time.Second / time.Duration(sampleRate*channels*2),
		}

		context, ready, err := oto.NewContext(op)
		if err != nil {
			globalOtoMutex.Unlock()
			return fmt.Errorf("failed to create oto context: %w", err)
		}

		<-ready
		globalContext = context
	}
	globalPlayers++
	context := globalContext
	globalOtoMutex.Unlock()

	// Create player with buffered reader
	s.player = context.NewPlayer(s.reader)
	s.closed = false

	// Start playing in background
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.player.Play()
	}()

	return nil
}

// Close closes the streaming output
func (s *StreamingOtoOutput) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	// Close writer first to signal EOF
	if s.writer != nil {
		s.writer.Close()
		s.writer = nil
	}

	// Wait a bit for buffer to flush
	time.Sleep(100 * time.Millisecond)

	// Close player
	if s.player != nil {
		s.player.Close()
		s.player = nil
	}

	// Close reader
	if s.reader != nil {
		s.reader.Close()
		s.reader = nil
	}

	// Decrease player count
	globalOtoMutex.Lock()
	globalPlayers--
	// Don't suspend context - keep it alive for reuse
	globalOtoMutex.Unlock()

	s.wg.Wait()
	return nil
}

// Write writes samples to the stream
func (s *StreamingOtoOutput) Write(samples []int16) error {
	s.mu.Lock()
	if s.closed || s.writer == nil {
		s.mu.Unlock()
		return fmt.Errorf("stream not open")
	}
	writer := s.writer
	s.mu.Unlock()

	// Convert int16 to bytes (little-endian)
	bytes := make([]byte, len(samples)*2)
	for i, sample := range samples {
		bytes[i*2] = byte(sample)
		bytes[i*2+1] = byte(sample >> 8)
	}

	// Write to pipe
	_, err := writer.Write(bytes)
	return err
}

// IsPlaying returns true if playing
func (s *StreamingOtoOutput) IsPlaying() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed && s.player != nil
}

// FallbackOutput uses time.Sleep for systems where audio doesn't work
type FallbackOutput struct {
	sampleRate int
	channels   int
	closed     bool
	mu         sync.Mutex
}

func NewFallbackOutput() (*FallbackOutput, error) {
	return &FallbackOutput{}, nil
}

func (f *FallbackOutput) Open(sampleRate, channels, bufferSize int) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sampleRate = sampleRate
	f.channels = channels
	f.closed = false
	return nil
}

func (f *FallbackOutput) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.closed = true
	return nil
}

func (f *FallbackOutput) Write(samples []int16) error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return fmt.Errorf("output closed")
	}
	sampleRate := f.sampleRate
	f.mu.Unlock()

	// Calculate duration and sleep
	duration := time.Duration(len(samples)) * time.Second / time.Duration(sampleRate)
	time.Sleep(duration)
	return nil
}

func (f *FallbackOutput) IsPlaying() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.closed
}
