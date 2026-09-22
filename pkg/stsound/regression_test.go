package stsound

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const benchmarkSong = "Goldrunner.ym"

func testSongPath(parts ...string) string {
	path := append([]string{"..", "..", "test", "testdata"}, parts...)
	return filepath.Join(path...)
}

// TestBundledCorpus loads and renders every checked-in YM fixture, including
// historical LH5 files whose redundant header-size field is unreliable.
func TestBundledCorpus(t *testing.T) {
	root := testSongPath()
	var failures []string
	loaded := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".ym") {
			return nil
		}

		player := CreateWithRate(44100)
		defer player.Destroy()
		if err := player.Load(path); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		loaded++
		buffer := make([]int16, 2048)
		if !player.Compute(buffer, len(buffer)) && player.GetInfo().MusicTimeInMs != 0 {
			return fmt.Errorf("%s ended during its first buffer", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if loaded < 968 || len(failures) != 0 {
		t.Fatalf("loaded only %d files; failures:\n%s", loaded, strings.Join(failures, "\n"))
	}
}

func TestLoadMemoryRejectsTruncatedFiles(t *testing.T) {
	tests := map[string][]byte{
		"empty":            nil,
		"YM2 header":       []byte("YM2!"),
		"YM3b header":      []byte("YM3b"),
		"YM5 signature":    []byte("YM5!LeOnArD!"),
		"YM6 short header": append([]byte("YM6!LeOnArD!"), make([]byte, 10)...),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			player := CreateWithRate(44100)
			defer player.Destroy()
			if err := player.LoadMemory(data); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestOptimizedRendererMatchesReferencePath(t *testing.T) {
	tests := []struct {
		name      string
		filter    YmBool
		registers map[YmInt]YmInt
	}{
		{
			name:   "tones and noise",
			filter: YmTrue,
			registers: map[YmInt]YmInt{
				0: 0x34, 1: 0x02, 2: 0x51, 3: 0x01, 4: 0x93, 5: 0x03,
				6: 0x0f, 7: 0x00, 8: 0x0d, 9: 0x0a, 10: 0x07,
			},
		},
		{
			name:   "envelope without filter",
			filter: YmFalse,
			registers: map[YmInt]YmInt{
				0: 0x20, 1: 0x01, 4: 0x71, 5: 0x02, 6: 0x08, 7: 0x12,
				8: 0x10, 9: 0x06, 10: 0x10, 11: 0x19, 12: 0x01, 13: 0x0e,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			optimized := NewYm2149Ex(ATARI_CLOCK, 1, 44100)
			reference := NewYm2149Ex(ATARI_CLOCK, 1, 44100)
			optimized.SetFilter(tt.filter)
			reference.SetFilter(tt.filter)
			for register, value := range tt.registers {
				optimized.WriteRegister(register, value)
				reference.WriteRegister(register, value)
			}
			// A zero-step sync buzzer selects nextSample without changing its PCM.
			reference.bSyncBuzzer = YmTrue

			for _, size := range []int{1, 127, 2048, 4097} {
				got := make([]YmSample, size)
				want := make([]YmSample, size)
				optimized.Update(got, YmInt(size))
				reference.Update(want, YmInt(size))
				if !slices.Equal(got, want) {
					for i := range got {
						if got[i] != want[i] {
							t.Fatalf("size %d differs at sample %d: got %d, want %d", size, i, got[i], want[i])
						}
					}
				}
			}
		})
	}
}

func hashSamples(samples []int16) string {
	h := sha256.New()
	var encoded [2]byte
	for _, sample := range samples {
		binary.LittleEndian.PutUint16(encoded[:], uint16(sample))
		_, _ = h.Write(encoded[:])
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// TestRenderRegression compares representative YM5/YM6 songs with SHA-256
// hashes generated independently by ST-Sound v1.43 at 44100 Hz, low-pass on,
// using 882-sample frames. Its signed timer unit and envelope denominator were
// widened in the reference harness to remove C++ overflow. Uneven Go buffers
// must produce the same PCM as the reference frame-aligned buffers.
func TestRenderRegression(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		songType   string
		songName   string
		durationMs uint32
		wantHash   string
	}{
		{
			name:       "YM5",
			path:       testSongPath("Hubbard Robb", benchmarkSong),
			songType:   "YM 5",
			songName:   "Goldrunner",
			durationMs: 188360,
			wantHash:   "1eb74ac81a8b02a2342dfdbb5590e497456ef1cab8886ab0d64fdb60bfc8ec21",
		},
		{
			name:       "YM5 digidrum",
			path:       testSongPath("Misc Demos", "Digizak.ym"),
			songType:   "YM 5",
			songName:   "Digizak",
			durationMs: 130580,
			wantHash:   "a2a08f275be2a4c3209e8f84322863bb4ae670f96f448dc8746ece27743d2db9",
		},
		{
			name:       "YM6 effects",
			path:       testSongPath("Misc Demos", "Synth Sample 1 01.ym"),
			songType:   "YM 6",
			songName:   "Synth Sample 1",
			durationMs: 133700,
			wantHash:   "e5c8308be55f875a4c1fb4db2c5d3c8b6b06ab7e2eb95951335efe6b22529970",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player := CreateWithRate(44100)
			defer player.Destroy()
			if err := player.Load(tt.path); err != nil {
				t.Fatal(err)
			}

			info := player.GetInfo()
			if info.SongType != tt.songType || info.SongName != tt.songName || uint32(info.MusicTimeInMs) != tt.durationMs {
				t.Fatalf("unexpected info: %+v", info)
			}

			const sampleCount = 10 * 44100
			const chunkSize = 1009
			samples := make([]int16, sampleCount)
			for offset := 0; offset < len(samples); {
				end := min(offset+chunkSize, len(samples))
				if !player.Compute(samples[offset:end], end-offset) {
					t.Fatalf("playback ended after %d samples", offset)
				}
				offset = end
			}

			got := hashSamples(samples)
			if got != tt.wantHash {
				t.Fatalf("PCM hash changed: got %s, want %s", got, tt.wantHash)
			}
		})
	}
}

func BenchmarkCompute(b *testing.B) {
	benchmarkCompute(b, testSongPath("Hubbard Robb", benchmarkSong))
}

func BenchmarkComputeDigiDrum(b *testing.B) {
	benchmarkCompute(b, testSongPath("Misc Demos", "Digizak.ym"))
}

func BenchmarkComputeYM6Effects(b *testing.B) {
	benchmarkCompute(b, testSongPath("Misc Demos", "Synth Sample 1 01.ym"))
}

func benchmarkCompute(b *testing.B, path string) {
	b.Helper()
	player := CreateWithRate(44100)
	defer player.Destroy()
	if err := player.Load(path); err != nil {
		b.Fatal(err)
	}
	player.SetLoopMode(true)
	buffer := make([]int16, 2048)

	b.ReportAllocs()
	b.SetBytes(int64(len(buffer) * 2))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		player.Compute(buffer, len(buffer))
	}
}

func BenchmarkLoadMemory(b *testing.B) {
	data, err := LoadYMFile(testSongPath("Hubbard Robb", benchmarkSong))
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		player := CreateWithRate(44100)
		if err := player.LoadMemory(data); err != nil {
			b.Fatal(err)
		}
	}
}
