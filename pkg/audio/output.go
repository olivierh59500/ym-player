package audio

import (
	"errors"
	"reflect"
	"sync"
	"time"
)

// Output interface for audio output implementations
type Output interface {
	Open(sampleRate, channels, bufferSize int) error
	Close() error
	Write(samples []int16) error
	IsPlaying() bool
}

// Player wraps the YM player with audio output
type Player struct {
	stSound    interface{} // *stsound.StSound
	output     Output
	sampleRate int
	bufferSize int
	playing    bool
	paused     bool
	mu         sync.Mutex
	done       chan bool
}

// NewPlayer creates a new audio player
func NewPlayer(stSound interface{}, output Output) *Player {
	return &Player{
		stSound: stSound,
		output:  output,
		done:    make(chan bool),
	}
}

// Start starts audio playback
func (p *Player) Start(sampleRate, bufferSize int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.playing {
		return errors.New("already playing")
	}
	
	p.sampleRate = sampleRate
	p.bufferSize = bufferSize
	
	// Open audio output
	if err := p.output.Open(sampleRate, 1, bufferSize); err != nil {
		return err
	}
	
	p.playing = true
	go p.audioLoop()
	
	return nil
}

// Stop stops audio playback
func (p *Player) Stop() {
	p.mu.Lock()
	if !p.playing {
		p.mu.Unlock()
		return
	}
	p.playing = false
	p.mu.Unlock()
	
	// Wait for audio loop to finish
	<-p.done
	
	// Close audio output
	p.output.Close()
}

// Pause pauses playback
func (p *Player) Pause() {
	p.mu.Lock()
	p.paused = true
	p.mu.Unlock()
}

// Resume resumes playback
func (p *Player) Resume() {
	p.mu.Lock()
	p.paused = false
	p.mu.Unlock()
}

// IsPaused returns true if paused
func (p *Player) IsPaused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.paused
}

// audioLoop is the main audio processing loop
func (p *Player) audioLoop() {
	defer func() {
		p.done <- true
	}()
	
	buffer := make([]int16, p.bufferSize)
	
	// Use reflection to call Compute method
	// In real implementation, use proper type assertion
	computeMethod := reflect.ValueOf(p.stSound).MethodByName("Compute")
	
	for {
		p.mu.Lock()
		if !p.playing {
			p.mu.Unlock()
			break
		}
		paused := p.paused
		p.mu.Unlock()
		
		if paused {
			// Write silence when paused
			for i := range buffer {
				buffer[i] = 0
			}
		} else {
			// Compute next audio samples
			args := []reflect.Value{
				reflect.ValueOf(buffer),
				reflect.ValueOf(len(buffer)),
			}
			result := computeMethod.Call(args)
			
			// Check if music is over
			if !result[0].Bool() {
				p.mu.Lock()
				p.playing = false
				p.mu.Unlock()
				break
			}
		}
		
		// Write to audio output
		if err := p.output.Write(buffer); err != nil {
			// Handle error
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// BufferOutput is a simple buffer-based output for testing
type BufferOutput struct {
	buffer     []int16
	sampleRate int
	channels   int
	mu         sync.Mutex
}

// NewBufferOutput creates a new buffer output
func NewBufferOutput() *BufferOutput {
	return &BufferOutput{}
}

// Open opens the buffer output
func (b *BufferOutput) Open(sampleRate, channels, bufferSize int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	b.sampleRate = sampleRate
	b.channels = channels
	b.buffer = make([]int16, 0, sampleRate*channels*10) // 10 seconds buffer
	return nil
}

// Close closes the buffer output
func (b *BufferOutput) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	b.buffer = nil
	return nil
}

// Write writes samples to the buffer
func (b *BufferOutput) Write(samples []int16) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if b.buffer == nil {
		return errors.New("buffer not initialized")
	}
	
	b.buffer = append(b.buffer, samples...)
	return nil
}

// IsPlaying always returns true for buffer output
func (b *BufferOutput) IsPlaying() bool {
	return true
}

// GetBuffer returns the accumulated audio buffer
func (b *BufferOutput) GetBuffer() []int16 {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	result := make([]int16, len(b.buffer))
	copy(result, b.buffer)
	return result
}

// Clear clears the buffer
func (b *BufferOutput) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	b.buffer = b.buffer[:0]
}