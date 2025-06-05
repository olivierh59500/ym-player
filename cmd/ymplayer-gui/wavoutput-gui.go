package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// WAVOutput writes audio to a WAV file (for GUI export)
type WAVOutput struct {
	file       *os.File
	filename   string
	sampleRate int
	channels   int
	written    int64
}

func (w *WAVOutput) Open(sampleRate, channels, bufferSize int) error {
	w.sampleRate = sampleRate
	w.channels = channels

	file, err := os.Create(w.filename)
	if err != nil {
		return err
	}

	w.file = file

	// Write WAV header
	header := make([]byte, 44)
	copy(header[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(header[4:8], 0) // File size - 8 (updated later)
	copy(header[8:12], []byte("WAVE"))
	copy(header[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(header[16:20], 16) // Format chunk size
	binary.LittleEndian.PutUint16(header[20:22], 1)  // Audio format (PCM)
	binary.LittleEndian.PutUint16(header[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	byteRate := sampleRate * channels * 2
	binary.LittleEndian.PutUint32(header[28:32], uint32(byteRate))
	blockAlign := channels * 2
	binary.LittleEndian.PutUint16(header[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(header[34:36], 16) // Bits per sample
	copy(header[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(header[40:44], 0) // Data size (updated later)

	_, err = w.file.Write(header)
	return err
}

func (w *WAVOutput) Close() error {
	if w.file == nil {
		return nil
	}

	// Update header with final sizes
	w.file.Seek(4, 0)
	binary.Write(w.file, binary.LittleEndian, uint32(w.written+36))
	
	w.file.Seek(40, 0)
	binary.Write(w.file, binary.LittleEndian, uint32(w.written))

	return w.file.Close()
}

func (w *WAVOutput) Write(samples []int16) error {
	if w.file == nil {
		return fmt.Errorf("file not open")
	}

	// Convert samples to bytes
	for _, sample := range samples {
		if err := binary.Write(w.file, binary.LittleEndian, sample); err != nil {
			return err
		}
		w.written += 2
	}

	return nil
}

func (w *WAVOutput) IsPlaying() bool {
	return false
}