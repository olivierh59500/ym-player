package stsound

import (
	//	"bytes"
	"fmt"
	"os"

	"ym-player/pkg/lzh"
)

// LoadYMFile loads a YM file from disk, handling both compressed and uncompressed formats
func LoadYMFile(filename string) ([]byte, error) {
	// Read file
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Check if it's compressed
	if lzh.IsLZHCompressed(data) {
		// Decompress
		decompressed, err := lzh.Decompress(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress LZH: %w", err)
		}
		return decompressed, nil
	}

	// Check if it's a valid YM file
	if !IsYMFile(data) {
		return nil, fmt.Errorf("not a valid YM file")
	}

	return data, nil
}

// IsYMFile checks if the data represents a valid YM file
func IsYMFile(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	// Check for YM signatures (big-endian)
	id := readBigEndian32(data[:4])

	switch id {
	case e_YM2a, e_YM3a, e_YM3b, e_YM4a, e_YM5a, e_YM6a, e_MIX1, e_YMT1, e_YMT2:
		return true
	}

	return false
}

// GetYMInfo returns basic information about a YM file without full loading
func GetYMInfo(data []byte) (format string, compressed bool, err error) {
	// Check if compressed
	if lzh.IsLZHCompressed(data) {
		compressed = true
		method := lzh.GetCompressionMethod(data)

		// Try to get info about the compressed content
		// For now, just return the compression method
		return fmt.Sprintf("Compressed YM (%s)", method), true, nil
	}

	// Check YM format
	if len(data) < 4 {
		return "", false, fmt.Errorf("data too small")
	}

	id := readBigEndian32(data[:4])

	switch id {
	case e_YM2a:
		format = "YM2!"
	case e_YM3a:
		format = "YM3!"
	case e_YM3b:
		format = "YM3b"
	case e_YM4a:
		format = "YM4!"
	case e_YM5a:
		format = "YM5!"
	case e_YM6a:
		format = "YM6!"
	case e_MIX1:
		format = "MIX1"
	case e_YMT1:
		format = "YMT1"
	case e_YMT2:
		format = "YMT2"
	default:
		return "", false, fmt.Errorf("unknown YM format: 0x%08X", id)
	}

	return format, false, nil
}

// AutoDetectAndLoad automatically detects the file format and loads it appropriately
func AutoDetectAndLoad(filename string) (*CYmMusic, error) {
	// Load file data
	data, err := LoadYMFile(filename)
	if err != nil {
		return nil, err
	}

	// Create YM player
	ym := NewYmMusic(44100)

	// Load the data
	if err := ym.LoadMemory(data); err != nil {
		return nil, err
	}

	return ym, nil
}
