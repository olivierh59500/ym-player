package stsound

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"slices"
	"testing"
)

func playbackMixFixture() []byte {
	var b bytes.Buffer
	b.WriteString("MIX1LeOnArD!")
	for _, v := range []uint32{0, 400, 1, 0, 400} {
		_ = binary.Write(&b, binary.BigEndian, v)
	}
	_ = binary.Write(&b, binary.BigEndian, uint16(2))
	_ = binary.Write(&b, binary.BigEndian, uint16(8000))
	b.WriteString("Mix\x00Author\x00Comment\x00")
	for i := 0; i < 400; i++ {
		b.WriteByte(byte(i))
	}
	return b.Bytes()
}

func playbackFixtures(t *testing.T) map[string][]byte {
	t.Helper()
	fixtures := map[string][]byte{"YMT1": trackerFixture(false), "MIX1": playbackMixFixture()}
	for name, path := range map[string]string{
		"YM5":       testSongPath("Hubbard Robb", benchmarkSong),
		"YM5 drums": testSongPath("Misc Demos", "Digizak.ym"),
		"YM6":       testSongPath("Misc Demos", "Synth Sample 1 01.ym"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		fixtures[name] = data
	}
	return fixtures
}

func TestPlaybackIndependentOfBufferSize(t *testing.T) {
	for name, data := range playbackFixtures(t) {
		for _, rate := range []int{44100, 48000} {
			t.Run(fmt.Sprintf("%s/%d", name, rate), func(t *testing.T) {
				var want []int16
				var wantPos uint32
				for _, chunk := range []int{rate * 2, 1, 137, 882, 1024} {
					p := CreateWithRate(rate)
					defer p.Destroy()
					if err := p.LoadMemory(data); err != nil {
						t.Fatal(err)
					}
					p.SetLoopMode(true)
					got := make([]int16, rate*2)
					for i := 0; i < len(got); i += chunk {
						end := min(i+chunk, len(got))
						p.Compute(got[i:end], end-i)
					}
					if want == nil {
						want, wantPos = got, p.GetPos()
					} else if !slices.Equal(got, want) || p.GetPos() != wantPos {
						t.Fatalf("PCM or position depends on buffer size %d", chunk)
					}
				}
			})
		}
	}
}

func TestRestartStopAndReloadClearPlaybackState(t *testing.T) {
	for name, data := range playbackFixtures(t) {
		t.Run(name, func(t *testing.T) {
			p := Create()
			defer p.Destroy()
			if err := p.LoadMemory(data); err != nil {
				t.Fatal(err)
			}
			p.SetLoopMode(true)
			want := make([]int16, 5003)
			p.Compute(want, len(want))
			for _, reset := range []func(){p.Restart, func() { p.Stop(); p.Play() }, func() {
				if err := p.LoadMemory(data); err != nil {
					t.Fatal(err)
				}
			}} {
				reset()
				if p.GetPos() != 0 || p.IsOver() {
					t.Fatal("reset retained position or end state")
				}
				got := make([]int16, len(want))
				p.Compute(got, len(got))
				if !slices.Equal(got, want) {
					t.Fatal("reset output differs from a fresh player")
				}
			}
		})
	}
}

func TestSeekReconstructsRegisterAndTrackerState(t *testing.T) {
	for name, data := range playbackFixtures(t) {
		if name == "MIX1" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			p := Create()
			defer p.Destroy()
			if err := p.LoadMemory(data); err != nil {
				t.Fatal(err)
			}
			p.SetLoopMode(true)
			p.Compute(make([]int16, 882), 882)
			want := make([]int16, 2003)
			p.Compute(want, len(want))
			p.Seek(20)
			got := make([]int16, len(want))
			p.Compute(got, len(got))
			if !slices.Equal(got, want) {
				t.Fatal("seek lost held notes or chip state")
			}
		})
	}
}

func TestTrackerPlaysFinalRowAndUsesLoopFrame(t *testing.T) {
	data := trackerFixture(false)
	const rate = 8000
	const rowSamples = rate / 50
	// The last row remains audible; earlier players stopped before mixing it.
	p := CreateWithRate(rate)
	defer p.Destroy()
	if err := p.LoadMemory(data); err != nil {
		t.Fatal(err)
	}
	out := make([]int16, 4*rowSamples+13)
	p.Compute(out, len(out))
	if out[3*rowSamples] != -32256 || out[4*rowSamples-1] != -32256 || !p.IsOver() {
		t.Fatal("tracker dropped its final row")
	}
	if !slices.Equal(out[4*rowSamples:], make([]int16, 13)) || p.Compute(out, len(out)) {
		t.Fatal("tracker did not pad and finish")
	}
	p.Restart()
	if !p.Compute(out[:3], 3) || out[0] != -32256 {
		t.Fatal("tracker cannot restart after EOF")
	}

	// Frame zero is silent, the declared loop at frame one is audible.
	binary.BigEndian.PutUint32(data[20:24], 1)
	start := len(data) - 16
	copy(data[start:start+4], []byte{255, 0, 0, 0})
	copy(data[start+4:start+8], []byte{0, 127, 0x1f, 0x40})
	if err := p.LoadMemory(data); err != nil {
		t.Fatal(err)
	}
	p.SetLoopMode(true)
	p.Compute(make([]int16, 4*rowSamples), 4*rowSamples)
	got := make([]int16, rowSamples)
	p.Compute(got, len(got))
	if got[0] != -32256 {
		t.Fatal("tracker ignored its declared loop frame")
	}
}

func TestRegisterFinalFrameAndYM2DrumMixer(t *testing.T) {
	frame := []byte{20, 1, 0, 0, 0, 0, 0, 0x3e, 15, 0, 0, 0, 0, 0xff, 0, 0}
	p := CreateWithRate(8000)
	defer p.Destroy()
	if err := p.LoadMemory(registerFixture("YM6!", 0, nil, frame)); err != nil {
		t.Fatal(err)
	}
	out := make([]int16, 173)
	p.Compute(out, len(out))
	if slices.Equal(out[:160], make([]int16, 160)) || !slices.Equal(out[160:], make([]int16, 13)) || !p.IsOver() {
		t.Fatal("register player did not render exactly its final frame")
	}
	frame[7], frame[10], frame[12] = 0, 0x80, 120
	if err := p.LoadMemory(append([]byte("YM2!"), frame[:14]...)); err != nil {
		t.Fatal(err)
	}
	p.Compute(out[:1], 1)
	if p.GetRegister(7)&0x24 != 0x24 {
		t.Fatal("YM2 drum did not disable channel C tone and noise")
	}
}

func TestComputeRejectsInvalidBufferCounts(t *testing.T) {
	p := Create()
	defer p.Destroy()
	for _, count := range []int{-1, 2} {
		if p.Compute(make([]int16, 1), count) {
			t.Fatal("invalid sample count accepted")
		}
	}
}

func TestNon50HzTimingPreservesReferenceIntervals(t *testing.T) {
	frame := []byte{20, 1, 0, 0, 0, 0, 0, 0x3e, 15, 0, 0, 0, 0, 0xff, 0, 0}
	for _, frameRate := range []uint16{56, 60, 100, 200} {
		for _, rate := range []int{44100, 48000} {
			t.Run(fmt.Sprintf("%d/%d", frameRate, rate), func(t *testing.T) {
				data := registerFixture("YM6!", 0, nil, frame, frame, frame)
				binary.BigEndian.PutUint16(data[26:28], frameRate)
				frameSamples := rate / int(frameRate)
				// Independent frame-aligned chip rendering is the reference
				// timing convention even when the division has a remainder.
				chip := NewYm2149Ex(ATARI_CLOCK, 1, YmU32(rate))
				want := make([]int16, frameSamples*3+13)
				for row := 0; row < 3; row++ {
					for reg := 0; reg < 13; reg++ {
						chip.WriteRegister(YmInt(reg), YmInt(frame[reg]))
					}
					chip.Update(want[row*frameSamples:(row+1)*frameSamples], YmInt(frameSamples))
				}
				for _, chunk := range []int{1, 137, 1024} {
					p := CreateWithRate(rate)
					defer p.Destroy()
					if err := p.LoadMemory(data); err != nil {
						t.Fatal(err)
					}
					got := make([]int16, len(want))
					for i := 0; i < len(got); i += chunk {
						end := min(i+chunk, len(got))
						p.Compute(got[i:end], end-i)
					}
					if !slices.Equal(got, want) || !p.IsOver() {
						t.Fatalf("timing differs for chunk size %d", chunk)
					}
				}
			})
		}
	}
}
