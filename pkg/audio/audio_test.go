package audio

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

func TestEncodePCM16LE(t *testing.T) {
	got := encodePCM16LE(nil, []int16{0, 1, -1, 0x1234, -0x1234})
	want := []byte{0, 0, 1, 0, 0xff, 0xff, 0x34, 0x12, 0xcc, 0xed}
	if !slices.Equal(got, want) {
		t.Fatalf("got %x, want %x", got, want)
	}
}

func TestWAVOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.wav")
	output := NewWAVOutput(path)
	if err := output.Open(44100, 1, 4); err != nil {
		t.Fatal(err)
	}
	if err := output.Write([]int16{0, 1}); err != nil {
		t.Fatal(err)
	}
	if err := output.Write([]int16{-1, 0x1234}); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 52 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		t.Fatalf("invalid WAV header: size=%d header=%q", len(data), data[:12])
	}
	if got := binary.LittleEndian.Uint32(data[4:8]); got != 44 {
		t.Fatalf("RIFF size = %d, want 44", got)
	}
	if got := binary.LittleEndian.Uint32(data[40:44]); got != 8 {
		t.Fatalf("data size = %d, want 8", got)
	}
	wantPCM := []byte{0, 0, 1, 0, 0xff, 0xff, 0x34, 0x12}
	if !slices.Equal(data[44:], wantPCM) {
		t.Fatalf("PCM data = %x, want %x", data[44:], wantPCM)
	}
}

type endingComputer struct {
	calls atomic.Int64
}

func (c *endingComputer) Compute(buffer []int16, nbSamples int) bool {
	c.calls.Add(1)
	return false
}

type countingOutput struct {
	opens  atomic.Int64
	closes atomic.Int64
}

func (o *countingOutput) Open(_, _, _ int) error {
	o.opens.Add(1)
	return nil
}
func (o *countingOutput) Close() error {
	o.closes.Add(1)
	return nil
}
func (o *countingOutput) Write([]int16) error { return nil }
func (o *countingOutput) IsPlaying() bool     { return true }

func TestPlayerCanRestartAfterNaturalEnd(t *testing.T) {
	computer := &endingComputer{}
	output := &countingOutput{}
	player := NewPlayer(computer, output)

	for run := 0; run < 2; run++ {
		if err := player.Start(44100, 128); err != nil {
			t.Fatal(err)
		}
		player.mu.Lock()
		done := player.done
		player.mu.Unlock()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("audio loop did not terminate")
		}
	}

	if computer.calls.Load() != 2 || output.opens.Load() != 2 || output.closes.Load() != 2 {
		t.Fatalf("calls=%d opens=%d closes=%d", computer.calls.Load(), output.opens.Load(), output.closes.Load())
	}
}

func BenchmarkEncodePCM16LE(b *testing.B) {
	samples := make([]int16, 2048)
	dst := make([]byte, len(samples)*2)
	b.ReportAllocs()
	b.SetBytes(int64(len(dst)))
	for i := 0; i < b.N; i++ {
		dst = encodePCM16LE(dst, samples)
	}
}
