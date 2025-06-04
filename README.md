# YM Player 🎵

A cross-platform YM music file player written in Go, supporting the Atari ST YM2149 sound chip music format.

![Go Version](https://img.shields.io/badge/Go-1.21%2B-blue)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey)
![License](https://img.shields.io/badge/license-BSD--2--Clause-green)

## Overview

YM Player is a modern implementation of the STSound library in Go, capable of playing YM music files from the Atari ST era. It faithfully emulates the YM2149 sound chip and supports various YM file formats, including compressed files.

### Features

- 🎮 **Accurate YM2149 emulation** - Faithful reproduction of the original sound chip
- 📦 **Multiple format support** - YM2!, YM3!, YM3b, YM5!, YM6!
- 🗜️ **LZH compression support** - Handles compressed YM files (LH0, LH4, LH5)
- 🔊 **Real-time audio playback** - Using Oto v3 for cross-platform audio
- 🎛️ **Audio controls** - Volume adjustment, looping, low-pass filter
- 💾 **WAV export** - Save YM files as WAV for use in other applications
- 🖥️ **Cross-platform** - Works on Windows, macOS, Linux (Intel/ARM)

## Installation

### Prerequisites

- Go 1.21 or higher
- C compiler (for CGo dependencies)

### Building from source

```bash
# Clone the repository
git clone https://github.com/olivierh59500/ym-player.git
cd ym-player

# Build the player
go build ./cmd/ymplayer

# Or install globally
go install ./cmd/ymplayer
```

## Usage

### Basic playback

```bash
# Play a YM file
./ymplayer music.ym

# Show file information only
./ymplayer -info music.ym
```

### Command-line options

```
Usage: ymplayer [options] <ym-file>

Options:
  -rate int
        Sample rate (Hz) (default 44100)
  -buffer int
        Buffer size (default 2048)
  -loop
        Loop playback
  -volume float
        Volume (0.0 to 10.0) (default 1.0)
  -gain float
        Audio gain multiplier (default 1.0)
  -lowpass
        Enable lowpass filter (default true)
  -info
        Show file info only
  -output string
        Output backend: oto, wav, null (default "oto")
  -wav string
        Output WAV file (when using wav output)
```

### Examples

```bash
# Play with double volume
./ymplayer -volume 2.0 music.ym

# Export to WAV file
./ymplayer -output wav -wav output.wav music.ym

# Play in loop with specific sample rate
./ymplayer -loop -rate 48000 music.ym

# Disable low-pass filter for sharper sound
./ymplayer -lowpass=false music.ym
```

## Supported Formats

### YM File Formats
- **YM2!** - Original YM format
- **YM3!** - YM3 format
- **YM3b** - YM3 with loop information
- **YM5!** - Extended format with metadata
- **YM6!** - Latest format with additional features

### Compression
- **Uncompressed** - Direct YM files
- **LH0** - Stored (no compression)
- **LH4** - LZ77 + Static Huffman
- **LH5** - LZ77 + Dynamic Huffman

## Project Structure

```
ym-player/
├── cmd/
│   └── ymplayer/       # Main player application
│       └── main.go
├── pkg/
│   ├── audio/          # Audio output interfaces
│   │   ├── output.go
│   │   └── oto.go
│   ├── lzh/            # LZH decompression
│   │   └── decoder.go
│   └── stsound/        # YM emulation core
│       ├── stsound.go  # Main API
│       ├── ym2149ex.go # YM2149 chip emulation
│       ├── ymmusic.go  # Music player logic
│       ├── ymload.go   # File loading
│       └── types.go    # Type definitions
├── go.mod
├── go.sum
└── README.md
```

## API Usage

### Basic Example

```go
package main

import (
    "log"
    "ym-player/pkg/stsound"
)

func main() {
    // Create player with 44.1kHz sample rate
    player := stsound.CreateWithRate(44100)
    defer player.Destroy()
    
    // Load YM file
    if err := player.Load("music.ym"); err != nil {
        log.Fatal(err)
    }
    
    // Get music info
    info := player.GetInfo()
    log.Printf("Title: %s", info.SongName)
    log.Printf("Author: %s", info.SongAuthor)
    
    // Start playback
    player.Play()
    
    // Generate audio samples
    buffer := make([]int16, 2048)
    for player.Compute(buffer, len(buffer)) {
        // Process audio buffer
        // Send to audio output...
    }
}
```

### Integration with Game Engines

See the [Ebiten integration example](docs/ebiten-integration.md) for using YM Player in game development.

## Technical Details

### YM2149 Emulation

The emulator accurately reproduces the behavior of the YM2149/AY-3-8910 sound chip:
- 3 square wave tone generators
- 1 noise generator
- 1 envelope generator
- Mixer controls for tone/noise
- Special effects (SID, DigiDrum, Sync-Buzzer)

### Architecture Support

The player correctly handles endianness differences:
- YM files use big-endian (Motorola 68000)
- LZH compression uses little-endian
- Automatic conversion for Intel/ARM architectures

## Building for Different Platforms

```bash
# Windows
GOOS=windows GOARCH=amd64 go build -o ymplayer.exe ./cmd/ymplayer

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o ymplayer-mac ./cmd/ymplayer

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o ymplayer-mac-arm64 ./cmd/ymplayer

# Linux
GOOS=linux GOARCH=amd64 go build -o ymplayer-linux ./cmd/ymplayer

# Linux (ARM)
GOOS=linux GOARCH=arm64 go build -o ymplayer-linux-arm64 ./cmd/ymplayer
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Development

```bash
# Run tests
go test ./...

# Run with race detector
go run -race ./cmd/ymplayer music.ym

# Profile CPU usage
go run ./cmd/ymplayer -cpuprofile=cpu.prof music.ym
go tool pprof cpu.prof
```

## Credits

- Original STSound library by Arnaud Carré (Leonard/Oxygene)
- YM file format by Leonard/Oxygene
- LZH decompression based on Haruhiko Okumura and Kerwin F. Medina's work
- Go port and enhancements by Olivier Houte

## License

This project is licensed under the BSD 2-Clause License - see the [LICENSE](LICENSE) file for details.

## Resources

- [YM Format Documentation](http://leonard.oxg.free.fr/ymformat.html)
- [Atari ST Sound Archive](http://sndh.atari.org/)
- [STSound Original Library](http://leonard.oxg.free.fr/stsound.html)

## Troubleshooting

### No sound output
- Check your system's audio settings
- Try increasing the buffer size: `-buffer 4096`
- Verify the YM file is not corrupted

### Choppy playback
- Increase buffer size: `-buffer 4096`
- Lower sample rate: `-rate 22050`
- Close other audio applications

### Cannot load file
- Ensure the file is a valid YM format
- Check if the file is compressed (look for LZH header)
- Try with a known working YM file

## Changelog

### v1.0.0 (2024-12-05)
- Initial release
- Full YM2149 emulation
- Support for YM2-YM6 formats
- LZH decompression support
- Cross-platform audio output
- WAV export functionality
