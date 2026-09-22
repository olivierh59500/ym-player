package stsound

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
)

func trackerCompatibilityFixture(version string, drums, rate uint16, loop uint32, note byte) []byte {
	var data bytes.Buffer
	data.WriteString(version + "LeOnArD!")
	for _, value := range []any{uint16(1), rate, uint32(4), loop, drums, uint32(0)} {
		_ = binary.Write(&data, binary.BigEndian, value)
	}
	data.WriteString("Sample bank\x00Author\x00Comment\x00")
	for i := uint16(0); i < drums; i++ {
		_ = binary.Write(&data, binary.BigEndian, uint16(1))
		if version == "YMT2" {
			_ = binary.Write(&data, binary.BigEndian, uint16(1))
			_ = binary.Write(&data, binary.BigEndian, uint16(0))
		}
		data.WriteByte(128)
	}
	for frame := 0; frame < 4; frame++ {
		frequency := uint16(8000)
		if drums == 0 {
			frequency = 0
		}
		data.Write([]byte{note, 127, byte(frequency >> 8), byte(frequency)})
	}
	return data.Bytes()
}

func TestDigitalTrackerBankDimensions(t *testing.T) {
	for _, version := range []string{"YMT1", "YMT2"} {
		for _, drums := range []uint16{0, 257, 65535} {
			t.Run(fmt.Sprintf("%s/%d-drums", version, drums), func(t *testing.T) {
				player := CreateWithRate(8000)
				defer player.Destroy()
				if err := player.LoadMemory(trackerCompatibilityFixture(version, drums, 50, 0, 0)); err != nil {
					t.Fatal(err)
				}
				if player.music.nbDrum != int(drums) {
					t.Fatalf("sample count: got %d, want %d", player.music.nbDrum, drums)
				}
				samples := make([]int16, 100)
				player.Compute(samples, len(samples))
				for i, value := range samples {
					if value != 0 {
						t.Fatalf("silent sample %d: got %d", i, value)
					}
				}
			})
		}
	}
}

func TestDigitalTrackerLoopAndFrameRate(t *testing.T) {
	for _, version := range []string{"YMT1", "YMT2"} {
		for _, loop := range []uint32{0, 2, 4, ^uint32(0)} {
			player := CreateWithRate(8000)
			if err := player.LoadMemory(trackerCompatibilityFixture(version, 1, 8000, loop, 0)); err != nil {
				t.Fatalf("%s loop=%d: %v", version, loop, err)
			}
			wantLoop := int(loop)
			if loop >= 4 {
				wantLoop = 0
			}
			if player.music.loopFrame != wantLoop {
				t.Fatalf("%s loop=%d: normalized to %d, want %d", version, loop, player.music.loopFrame, wantLoop)
			}
			if player.music.playerRate != 8000 {
				t.Fatalf("%s: frame rate was altered", version)
			}
			player.Destroy()
		}
		player := CreateWithRate(8000)
		if err := player.LoadMemory(trackerCompatibilityFixture(version, 1, 8001, 0, 0)); err == nil {
			t.Fatalf("%s accepted a frame rate above the audio rate", version)
		}
		player.Destroy()
	}
}

func TestDigitalTrackerRejectsMissingSampleReference(t *testing.T) {
	for _, version := range []string{"YMT1", "YMT2"} {
		player := Create()
		defer player.Destroy()
		if err := player.LoadMemory(trackerCompatibilityFixture(version, 1, 50, 0, 1)); err == nil {
			t.Fatalf("%s accepted a note outside the sample bank", version)
		}
	}
}

func mixCompatibilityFixture(samples []byte, start, length uint32, repeat, rate uint16) []byte {
	var data bytes.Buffer
	data.WriteString("MIX1LeOnArD!")
	for _, value := range []any{uint32(1), uint32(len(samples)), uint32(1), start, length, repeat, rate} {
		_ = binary.Write(&data, binary.BigEndian, value)
	}
	data.WriteString("Mix sample\x00Author\x00Comment\x00")
	data.Write(samples)
	return data.Bytes()
}

func TestDigitalMixLongBlock(t *testing.T) {
	const length = 1<<20 + 3
	samples := make([]byte, length)
	samples[0], samples[length-1] = 0x80, 0x7f
	player := CreateWithRate(8000)
	defer player.Destroy()
	if err := player.LoadMemory(mixCompatibilityFixture(samples, 0, length, 1, 8000)); err != nil {
		t.Fatal(err)
	}
	output := make([]int16, length+2)
	player.Compute(output, len(output))
	for i, got := range output {
		var want int16
		if i == 0 {
			want = -32768
		} else if i == length-1 {
			want = 32512
		}
		if got != want {
			t.Fatalf("sample %d: got %d, want %d", i, got, want)
		}
	}
	if !player.IsOver() {
		t.Fatal("long MIX1 block did not end")
	}
}

func TestDigitalMixRejectsInvalidBlockBounds(t *testing.T) {
	for name, block := range map[string]struct {
		start, length uint32
		repeat, rate  uint16
	}{
		"start outside buffer": {4, 1, 1, 8000},
		"end outside buffer":   {2, 3, 1, 8000},
		"integer wrap":         {^uint32(0), 2, 1, 8000},
		"empty block":          {0, 0, 1, 8000},
		"zero repeat":          {0, 1, 0, 8000},
		"zero rate":            {0, 1, 1, 0},
	} {
		t.Run(name, func(t *testing.T) {
			player := Create()
			defer player.Destroy()
			data := mixCompatibilityFixture(make([]byte, 4), block.start, block.length, block.repeat, block.rate)
			if err := player.LoadMemory(data); err == nil {
				t.Fatal("accepted an invalid MIX1 block")
			}
		})
	}
}

func TestDigitalMixRejectsUnrepresentableRate(t *testing.T) {
	for _, rate := range []uint16{1, 10} {
		player := CreateWithRate(44100)
		err := player.LoadMemory(mixCompatibilityFixture([]byte{127}, 0, 1, 1, rate))
		player.Destroy()
		if err == nil {
			t.Fatalf("accepted MIX1 rate %d with a zero playback increment", rate)
		}
	}

	// Eleven hertz is the first rate representable in Q12 at 44.1 kHz.
	player := CreateWithRate(44100)
	defer player.Destroy()
	if err := player.LoadMemory(mixCompatibilityFixture([]byte{127}, 0, 1, 1, 11)); err != nil {
		t.Fatal(err)
	}
	output := make([]int16, 4097)
	player.Compute(output, len(output))
	for i, value := range output {
		want := int16(32512)
		if i == 4096 {
			want = 0
		}
		if value != want {
			t.Fatalf("sample %d: got %d, want %d", i, value, want)
		}
	}
	if !player.IsOver() {
		t.Fatal("lowest representable MIX1 rate failed to finish")
	}
}

func TestDigitalCompatibilityTruncatedHeadersAndSamples(t *testing.T) {
	for name, fixture := range map[string][]byte{
		"YMT2": trackerCompatibilityFixture("YMT2", 1, 50, 0, 0),
		"MIX1": mixCompatibilityFixture([]byte{0, 127, 128}, 0, 3, 1, 8000),
	} {
		t.Run(name, func(t *testing.T) {
			for size := 0; size < len(fixture); size++ {
				player := Create()
				err := player.LoadMemory(fixture[:size])
				player.Destroy()
				if err == nil {
					t.Fatalf("accepted truncation at %d/%d bytes", size, len(fixture))
				}
			}
		})
	}
}
