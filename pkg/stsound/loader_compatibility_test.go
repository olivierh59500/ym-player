package stsound

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func registerFixture(signature string, flags uint32, drums [][]byte, frames ...[]byte) []byte {
	var data bytes.Buffer
	data.WriteString(signature)
	data.WriteString("LeOnArD!")
	_ = binary.Write(&data, binary.BigEndian, uint32(len(frames)))
	_ = binary.Write(&data, binary.BigEndian, flags)
	_ = binary.Write(&data, binary.BigEndian, uint16(len(drums)))
	_ = binary.Write(&data, binary.BigEndian, uint32(ATARI_CLOCK))
	_ = binary.Write(&data, binary.BigEndian, uint16(50))
	_ = binary.Write(&data, binary.BigEndian, uint32(0))
	_ = binary.Write(&data, binary.BigEndian, uint16(3))
	data.Write([]byte{1, 2, 3}) // Unknown extension bytes must be skipped.
	for _, drum := range drums {
		_ = binary.Write(&data, binary.BigEndian, uint32(len(drum)))
		data.Write(drum)
	}
	data.WriteString("Title\x00Author\x00Comment\x00")
	if flags&A_STREAMINTERLEAVED != 0 {
		for register := 0; register < 16; register++ {
			for _, frame := range frames {
				data.WriteByte(frame[register])
			}
		}
	} else {
		for _, frame := range frames {
			data.Write(frame)
		}
	}
	data.WriteString("End!")
	return data.Bytes()
}

func TestLoadRegisterFormatsAndLayouts(t *testing.T) {
	first, second := make([]byte, 16), make([]byte, 16)
	for i := range first {
		first[i], second[i] = byte(i), byte(31-i)
	}
	for _, signature := range []string{"YM2!", "YM3!", "YM3b", "YM5!", "YM6!"} {
		for _, interleaved := range []bool{false, true} {
			if signature < "YM5!" && !interleaved {
				continue // Early formats always use register planes.
			}
			t.Run(signature+"/interleaved="+map[bool]string{false: "false", true: "true"}[interleaved], func(t *testing.T) {
				var data, want []byte
				if signature < "YM5!" {
					data = []byte(signature)
					for register := 0; register < 14; register++ {
						data = append(data, first[register], second[register])
					}
					if signature == "YM3b" {
						data = binary.LittleEndian.AppendUint32(data, 1)
					}
					want = append(bytes.Clone(first[:14]), second[:14]...)
				} else {
					flags := uint32(0)
					if interleaved {
						flags = A_STREAMINTERLEAVED
					}
					data = registerFixture(signature, flags, nil, first, second)
					want = append(bytes.Clone(first), second...)
				}
				player := NewYmMusic(44100)
				if err := player.LoadMemory(data); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(player.pDataStream, want) {
					t.Fatalf("wrong register layout: %x", player.pDataStream)
				}
				if player.nbFrame != 2 {
					t.Fatalf("wrong frame count: %d", player.nbFrame)
				}
				if signature == "YM3b" && player.loopFrame != 1 {
					t.Fatalf("wrong little-endian loop: %d", player.loopFrame)
				}
				clear(data)
				if !bytes.Equal(player.pDataStream, want) {
					t.Fatal("LoadMemory retained mutable caller data")
				}
			})
		}
	}
}

func TestLoadFullUint16DigiDrumTable(t *testing.T) {
	data := registerFixture("YM6!", 0, make([][]byte, 65535), make([]byte, 16))
	player := NewYmMusic(44100)
	if err := player.LoadMemory(data); err != nil {
		t.Fatal(err)
	}
	if len(player.pDrumTab) != 65535 {
		t.Fatalf("lost digidrum entries: %d", len(player.pDrumTab))
	}
}

func TestFourBitDigiDrumConversionAndOwnership(t *testing.T) {
	samples := make([]byte, 17)
	for i := 0; i < 16; i++ {
		samples[i] = byte(i)
	}
	samples[16] = 0xff
	data := registerFixture("YM5!", A_DRUM4BITS|A_DRUMSIGNED, [][]byte{samples}, make([]byte, 16))
	original := bytes.Clone(data)
	player := NewYmMusic(44100)
	if err := player.LoadMemory(data); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, original) {
		t.Fatal("digidrum conversion changed the caller's input")
	}
	// Independent full-scale conversion from the original ST-Sound loader.
	want := []YmU8{0, 1, 2, 2, 4, 6, 9, 12, 17, 24, 35, 48, 72, 103, 165, 255, 255}
	for i, sample := range want {
		if player.pDrumTab[0].Data[i] != sample {
			t.Fatalf("sample %d: got %d, want %d", i, player.pDrumTab[0].Data[i], sample)
		}
	}
	if player.attrib&A_DRUM4BITS != 0 {
		t.Fatal("converted samples still marked four-bit")
	}
	clear(data)
	if player.pDrumTab[0].Data[2] != want[2] {
		t.Fatal("drum still aliases input")
	}
}

func TestLoadRejectsInvalidRegisterHeadersAndClearsState(t *testing.T) {
	valid := registerFixture("YM6!", 0, nil, make([]byte, 16))
	for name, mutate := range map[string]func([]byte) []byte{
		"zero rate":           func(b []byte) []byte { binary.BigEndian.PutUint16(b[26:28], 0); return b },
		"rate above output":   func(b []byte) []byte { binary.BigEndian.PutUint16(b[26:28], 44101); return b },
		"zero clock":          func(b []byte) []byte { binary.BigEndian.PutUint32(b[22:26], 0); return b },
		"zero frames":         func(b []byte) []byte { binary.BigEndian.PutUint32(b[12:16], 0); return b },
		"truncated drums":     func(b []byte) []byte { binary.BigEndian.PutUint16(b[20:22], 65535); return b },
		"truncated extension": func(b []byte) []byte { binary.BigEndian.PutUint16(b[32:34], 65535); return b },
		"truncated metadata":  func(b []byte) []byte { return b[:40] },
		"truncated frames":    func(b []byte) []byte { return b[:len(b)-5] },
	} {
		t.Run(name, func(t *testing.T) {
			player := NewYmMusic(44100)
			if err := player.LoadMemory(valid); err != nil {
				t.Fatal(err)
			}
			if err := player.LoadMemory(mutate(bytes.Clone(valid))); err == nil {
				t.Fatal("invalid header accepted")
			}
			if player.bMusicOk || player.nbFrame != 0 || player.pSongName != "" || len(player.pDataStream) != 0 || len(player.pDrumTab) != 0 {
				t.Fatal("failed load retained partial or stale song state")
			}
		})
	}
}

func TestLoadNormalizesLegacyInvalidLoop(t *testing.T) {
	for _, loop := range []uint32{1, ^uint32(0)} {
		data := registerFixture("YM6!", 0, nil, make([]byte, 16))
		binary.BigEndian.PutUint32(data[28:32], loop)
		player := NewYmMusic(44100)
		if err := player.LoadMemory(data); err != nil {
			t.Fatal(err)
		}
		if player.loopFrame != 0 {
			t.Fatalf("invalid legacy loop %d was not reset", loop)
		}
	}
}

func TestYM4IsRecognizedButUnsupportedLikeSTSound(t *testing.T) {
	if !IsYMFile([]byte("YM4!")) {
		t.Fatal("YM4 signature was not recognized")
	}
	if err := NewYmMusic(44100).LoadMemory([]byte("YM4!")); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unexpected YM4 result: %v", err)
	}
}

func TestLoadYMFileRejectsCompressedNonYMData(t *testing.T) {
	payload := []byte("not a YM stream")
	data := make([]byte, 24+len(payload))
	data[0] = 22
	copy(data[2:7], "-lh0-")
	binary.LittleEndian.PutUint32(data[7:11], uint32(len(payload)))
	binary.LittleEndian.PutUint32(data[11:15], uint32(len(payload)))
	copy(data[24:], payload)
	path := filepath.Join(t.TempDir(), "not-music.ym")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadYMFile(path); err == nil {
		t.Fatal("compressed non-YM payload was accepted")
	}
}

func TestLegacyRegisterFormatsRejectOutputRateBelowFrameRate(t *testing.T) {
	for _, signature := range []string{"YM2!", "YM3!", "YM3b"} {
		t.Run(signature, func(t *testing.T) {
			data := append([]byte(signature), make([]byte, 14)...)
			if signature == "YM3b" {
				data = binary.LittleEndian.AppendUint32(data, 0)
			}
			if err := NewYmMusic(49).LoadMemory(data); err == nil {
				t.Fatal("output rate below the fixed 50 Hz player rate was accepted")
			}
			player := NewYmMusic(50)
			if err := player.LoadMemory(data); err != nil {
				t.Fatalf("one output sample per frame must remain valid: %v", err)
			}
			player.Update(make([]YmSample, 1), 1)
		})
	}
}
