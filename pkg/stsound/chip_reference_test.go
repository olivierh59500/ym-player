package stsound

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

// These PCM digests were generated from the supplied ST-Sound C++ chip, with
// its signed SID phase literal changed from 1<<31 to 1u<<31 to avoid signed
// overflow. Ordinary tones, noise, envelopes and drums use unmodified code.
// Keeping only the digests makes the tests independent of a C++ toolchain.
func TestChipMatchesSTSoundReference(t *testing.T) {
	hashes := []string{
		"9380ec2d98120163b6b8281542b5bafa1ff4d2917f6de8e07b3ceefd2140ab0d",
		"9380ec2d98120163b6b8281542b5bafa1ff4d2917f6de8e07b3ceefd2140ab0d",
		"9380ec2d98120163b6b8281542b5bafa1ff4d2917f6de8e07b3ceefd2140ab0d",
		"9380ec2d98120163b6b8281542b5bafa1ff4d2917f6de8e07b3ceefd2140ab0d",
		"9be70152705f120c63a34d36994fc0d6d4559a002de32824688efc8a127a4696",
		"9be70152705f120c63a34d36994fc0d6d4559a002de32824688efc8a127a4696",
		"9be70152705f120c63a34d36994fc0d6d4559a002de32824688efc8a127a4696",
		"9be70152705f120c63a34d36994fc0d6d4559a002de32824688efc8a127a4696",
		"70273cd6e7a8bb8e4e4e10024e5a710efb84fd4ffe6c20192fd6974c5df84312",
		"9380ec2d98120163b6b8281542b5bafa1ff4d2917f6de8e07b3ceefd2140ab0d",
		"e6fa486dd5096bb933ecce7dcaf24ffae26c5bdd269572c796add8120797ae8c",
		"eef484f3ec9641d0ada19da0f9effa3a783d79042b5591697bec3f1385a18652",
		"a6668768c85d9d5f22be2e82ef75d07911ddb08924b5599530a5f5bfcae7812c",
		"b862e546ae13da42eec87810f30551a60a33abcd28f3239a5a76174215506b07",
		"2e9eb41e963c7313706e048de7121b5c1782859a7332845d96ff54e7429a16b3",
		"9be70152705f120c63a34d36994fc0d6d4559a002de32824688efc8a127a4696",
		"1aa926f0081df62886d049d4ee33672585a0c449d3079a38372228f75a4029f3",
		"c5b36cfdb44525f9e691d1353c8395e4f3310ab1327ed7dbaa312ad01d2ce51e",
		"71978a67b3105a308622ebc39dc7edadc1c621dec35ad1797766d5f3b1a26677",
	}
	for which, want := range hashes {
		t.Run(fmt.Sprintf("case-%02d", which), func(t *testing.T) {
			chip := NewYm2149Ex(ATARI_CLOCK, 1, 44100)
			registers := []YmInt{0x34, 2, 0x51, 1, 0x93, 3, 15, 0, 16, 16, 16, 37, 0, YmInt(which % 16)}
			for register, value := range registers {
				chip.WriteRegister(YmInt(register), value)
			}
			switch which {
			case 16:
				chip.SidStart(0, 1000, 15)
			case 17:
				chip.SyncBuzzerStart(1000, 10)
			case 18:
				drum := make([]YmU8, 256)
				for i := range drum {
					drum[i] = YmU8(i * 31)
				}
				chip.DrumStart(0, drum, YmU32(len(drum)), 22050)
			}
			samples := make([]YmSample, 8192)
			// Uneven calls also cross from an active drum to the normal fast path.
			for start := 0; start < len(samples); start += 137 {
				end := min(start+137, len(samples))
				chip.Update(samples[start:end], YmInt(end-start))
			}
			if got := hashSamples(samples); got != want {
				t.Fatalf("PCM differs from ST-Sound: got %s, want %s", got, want)
			}
		})
	}
}

func TestChipEnvelopeShapes(t *testing.T) {
	// d/u are descending/ascending ramps; z/h hold zero/the maximum level.
	shapes := []string{
		"dzzz", "dzzz", "dzzz", "dzzz", "uzzz", "uzzz", "uzzz", "uzzz",
		"dddd", "dzzz", "dudu", "dhhh", "uuuu", "uhhh", "udud", "uzzz",
	}
	for shape, segments := range shapes {
		for step := 0; step < 64; step++ {
			var want YmU8
			switch segments[step/16] {
			case 'd':
				want = YmU8(15 - step%16)
			case 'u':
				want = YmU8(step % 16)
			case 'h':
				want = 15
			}
			if got := envelopeData[shape][step/32][step%32]; got != want {
				t.Fatalf("shape %d step %d: got %d, want %d", shape, step, got, want)
			}
		}
	}
}

func TestChipSIDPhaseContinuesWhenDisabled(t *testing.T) {
	for _, effectsPath := range []bool{false, true} {
		chip := NewYm2149Ex(ATARI_CLOCK, 1, 44100)
		chip.SidStart(1, 1000, 15)
		const step = uint32(1000 * ((1 << 31) / 44100))
		if got := uint32(chip.specialEffect[1].SidStep); got != step {
			t.Fatalf("SID increment: got %d, want %d", got, step)
		}
		chip.Update(make([]YmSample, 37), 37)
		chip.SidStop(1)
		if effectsPath {
			chip.SyncBuzzerStart(0, 0)
		}
		chip.Update(make([]YmSample, 23), 23)
		chip.SidStart(1, 1000, 15)
		chip.Update(make([]YmSample, 1), 1)
		if got, want := chip.specialEffect[1].SidPos, YmU32(step*61); got != want {
			t.Fatalf("effects=%t: SID phase got %d, want %d", effectsPath, got, want)
		}
		if got := chip.ReadRegister(9); got != 15 {
			t.Fatalf("effects=%t: SID should resume in the high half-cycle, got volume %d", effectsPath, got)
		}
	}
}

func TestChipSyncBuzzerResetsEnvelope(t *testing.T) {
	chip := NewYm2149Ex(ATARI_CLOCK, 1, 44100)
	chip.WriteRegister(11, 37)
	chip.SyncBuzzerStart(1000, 10)
	chip.Update(make([]YmSample, 44), 44)
	if chip.envPos == 0 {
		t.Fatal("envelope unexpectedly reset before the timer period")
	}
	chip.Update(make([]YmSample, 1), 1)
	if chip.envPos != 0 || chip.envPhase != 0 {
		t.Fatalf("sync buzzer did not reset the envelope: position=%d phase=%d", chip.envPos, chip.envPhase)
	}
}

func TestChipLargeEnvelopePeriods(t *testing.T) {
	for _, rate := range []YmU32{44100, 48000, 96000, 192000} {
		chip := NewYm2149Ex(ATARI_CLOCK, 1, rate)
		chip.WriteRegister(11, 255)
		chip.WriteRegister(12, 255)
		want := YmU32((uint64(ATARI_CLOCK) << 23) / (65535 * uint64(rate)))
		if chip.envStep != want {
			t.Fatalf("rate=%d: envelope step got %d, want %d", rate, chip.envStep, want)
		}
	}
}

func TestChipDrumHighRateAndLongSample(t *testing.T) {
	chip := NewYm2149Ex(ATARI_CLOCK, 1, 44100)
	const size = 140000
	drum := make([]YmU8, size)
	for i := range drum {
		drum[i] = YmU8(i)
	}
	chip.DrumStart(0, drum, size, 88200)
	if chip.specialEffect[0].DrumStep != 2<<DRUM_PREC {
		t.Fatalf("overflow in high-rate drum increment: %d", chip.specialEffect[0].DrumStep)
	}
	chip.Update(make([]YmSample, size/2), size/2)
	if chip.specialEffect[0].Drum || chip.effectMask != 0 {
		t.Fatal("drum longer than 128 KiB wrapped instead of ending")
	}
	if got, want := chip.specialEffect[0].DrumPos, uint64(size)<<DRUM_PREC; got != want {
		t.Fatalf("drum final position: got %d, want %d", got, want)
	}
}

func TestChipEffectInputBounds(t *testing.T) {
	chip := NewYm2149Ex(ATARI_CLOCK, 1, 44100)
	for _, voice := range []YmInt{-1, 3} {
		chip.DrumStart(voice, []YmU8{128}, 1, 44100)
		chip.DrumStop(voice)
		chip.SidStart(voice, 1000, 15)
		chip.SidStop(voice)
	}
	chip.DrumStart(0, []YmU8{128}, 100, 44100)
	chip.Update(make([]YmSample, 2), 2)
	if chip.specialEffect[0].Drum {
		t.Fatal("drum declared length was not limited to the provided buffer")
	}
}

func TestChipBuiltinDrumsMatchSTSound(t *testing.T) {
	hash := sha256.New()
	for i, sample := range sampleAddress {
		if len(sample) != int(sampleLen[i]) {
			t.Fatalf("drum %d has %d bytes, want %d", i, len(sample), sampleLen[i])
		}
		bytes := make([]byte, len(sample))
		for j, value := range sample {
			bytes[j] = byte(value)
		}
		_, _ = hash.Write(bytes)
	}
	const want = "a142bd825998cb99b360e69765a89895fd93d7e4da93fa7b4e3b465daa062f3d"
	if got := fmt.Sprintf("%x", hash.Sum(nil)); got != want {
		t.Fatalf("YM2 drum bank differs from ST-Sound: got %s, want %s", got, want)
	}
}
