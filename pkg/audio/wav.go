package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

// WAVOutput writes interleaved signed 16-bit PCM samples to a WAV file.
type WAVOutput struct {
	file     *os.File
	filename string
	written  uint64
	pcm      []byte
}

func NewWAVOutput(filename string) *WAVOutput {
	return &WAVOutput{filename: filename}
}

func (w *WAVOutput) Open(sampleRate, channels, _ int) error {
	if w.file != nil {
		return errors.New("WAV output already open")
	}
	if sampleRate <= 0 || channels <= 0 {
		return fmt.Errorf("invalid WAV format: %d Hz, %d channels", sampleRate, channels)
	}

	file, err := os.Create(w.filename)
	if err != nil {
		return err
	}

	w.written = 0
	w.file = file

	var header [44]byte
	copy(header[0:4], "RIFF")
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(sampleRate*channels*2))
	binary.LittleEndian.PutUint16(header[32:34], uint16(channels*2))
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], "data")

	if _, err := file.Write(header[:]); err != nil {
		_ = file.Close()
		w.file = nil
		return err
	}
	return nil
}

func (w *WAVOutput) Close() error {
	if w.file == nil {
		return nil
	}
	file := w.file
	w.file = nil

	if w.written > uint64(^uint32(0))-36 {
		_ = file.Close()
		return errors.New("WAV output exceeds the 4 GiB RIFF size limit")
	}

	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(w.written+36))
	if _, err := file.WriteAt(size[:], 4); err != nil {
		_ = file.Close()
		return err
	}
	binary.LittleEndian.PutUint32(size[:], uint32(w.written))
	if _, err := file.WriteAt(size[:], 40); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (w *WAVOutput) Write(samples []int16) error {
	if w.file == nil {
		return errors.New("WAV output not open")
	}

	w.pcm = encodePCM16LE(w.pcm, samples)

	n, err := w.file.Write(w.pcm)
	w.written += uint64(n)
	return err
}

func (w *WAVOutput) IsPlaying() bool {
	return w.file != nil
}
