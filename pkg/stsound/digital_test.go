package stsound

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

func trackerFixture(interleaved bool) []byte {
	var b bytes.Buffer
	b.WriteString("YMT1LeOnArD!")
	put := func(v any) {
		if err := binary.Write(&b, binary.BigEndian, v); err != nil {
			panic(err)
		}
	}
	put(uint16(1))
	put(uint16(50))
	put(uint32(4))
	put(uint32(0))
	put(uint16(1))
	flags := uint32(0)
	if interleaved {
		flags = 1
	}
	put(flags)
	b.WriteString("Test\x00Author\x00Unsigned samples\x00")
	put(uint16(3))
	b.Write([]byte{0, 128, 255})
	data := []byte{0, 127, 0x1f, 0x40, 255, 127, 0x1f, 0x40, 255, 127, 0x1f, 0x40, 255, 127, 0x1f, 0x40}
	if interleaved {
		for column := 0; column < 4; column++ {
			for frame := 0; frame < 4; frame++ {
				b.WriteByte(data[frame*4+column])
			}
		}
	} else {
		b.Write(data)
	}
	return b.Bytes()
}

func TestTrackerInterleavingPCMAndInputOwnership(t *testing.T) {
	for _, interleaved := range []bool{false, true} {
		data := trackerFixture(interleaved)
		original := bytes.Clone(data)
		p := CreateWithRate(8000)
		if err := p.LoadMemory(data); err != nil {
			t.Fatal(err)
		}
		p.SetLoopMode(true)
		out := make([]int16, 240)
		if !p.Compute(out, len(out)) {
			t.Fatal("tracker stopped")
		}
		want := []int16{-32256, 0, 32004}
		for i, v := range out {
			if v != want[i%3] {
				t.Fatalf("interleaved=%v sample %d: %d != %d", interleaved, i, v, want[i%3])
			}
		}
		if !bytes.Equal(data, original) {
			t.Fatal("LoadMemory mutated the input")
		}
		p.Destroy()
	}
}

func TestDigitalLoaderRejectsTruncation(t *testing.T) {
	data := trackerFixture(true)
	for i := 0; i < len(data); i++ {
		p := Create()
		if err := p.LoadMemory(data[:i]); err == nil {
			t.Fatalf("accepted truncated YMT at %d", i)
		}
		p.Destroy()
	}
}

func TestMixSignedConversionAndEndPadding(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("MIX1LeOnArD!")
	for _, v := range []uint32{0, 3, 1, 0, 3} {
		binary.Write(&b, binary.BigEndian, v)
	}
	binary.Write(&b, binary.BigEndian, uint16(1))
	binary.Write(&b, binary.BigEndian, uint16(8000))
	b.WriteString("Mix\x00Author\x00Comment\x00")
	b.Write([]byte{0, 128, 255})
	p := CreateWithRate(8000)
	defer p.Destroy()
	if err := p.LoadMemory(b.Bytes()); err != nil {
		t.Fatal(err)
	}
	out := []int16{99, 99, 99, 99, 99}
	p.Compute(out, len(out))
	want := []int16{-32768, 0, 32512, 0, 0}
	for i, v := range out {
		if v != want[i] {
			t.Fatal(out)
		}
	}
	if p.Compute(out, len(out)) {
		t.Fatal("MIX1 did not finish")
	}
}

func TestKnucklebusterTrackerIsIndependentOfReadBlockSize(t *testing.T) {
	data, err := os.ReadFile("testdata/cuddly-knucklebuster.ym")
	if err != nil {
		t.Fatal(err)
	}
	a, b := CreateWithRate(48000), CreateWithRate(48000)
	defer a.Destroy()
	defer b.Destroy()
	for _, p := range []*StSound{a, b} {
		if err = p.LoadMemory(data); err != nil {
			t.Fatal(err)
		}
		p.SetLoopMode(true)
		if p.GetInfo().SongType != "YM-T1" || p.GetInfo().MusicTimeInMs != 1095000 {
			t.Fatal(p.GetInfo())
		}
	}
	x, y := make([]int16, 96000), make([]int16, 96000)
	a.Compute(x, len(x))
	for i := 0; i < len(y); {
		n := min(137, len(y)-i)
		b.Compute(y[i:i+n], n)
		i += n
	}
	nonzero := 0
	for i := range x {
		if x[i] != y[i] {
			t.Fatalf("block-dependent sample %d", i)
		}
		if x[i] != 0 {
			nonzero++
		}
	}
	if nonzero == 0 {
		t.Fatal("silent Knucklebuster output")
	}
}
