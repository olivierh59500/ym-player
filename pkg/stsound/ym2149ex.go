package stsound

// Envelope shapes
var (
	env00xx = []YmInt{1, 0, 0, 0, 0, 0, 0, 0}
	env01xx = []YmInt{0, 1, 0, 0, 0, 0, 0, 0}
	env1000 = []YmInt{1, 0, 1, 0, 1, 0, 1, 0}
	env1001 = []YmInt{1, 0, 0, 0, 0, 0, 0, 0}
	env1010 = []YmInt{1, 0, 0, 1, 1, 0, 0, 1}
	env1011 = []YmInt{1, 0, 1, 1, 1, 1, 1, 1}
	env1100 = []YmInt{0, 1, 0, 1, 0, 1, 0, 1}
	env1101 = []YmInt{0, 1, 1, 1, 1, 1, 1, 1}
	env1110 = []YmInt{0, 1, 1, 0, 0, 1, 1, 0}
	env1111 = []YmInt{0, 1, 0, 0, 0, 0, 0, 0}

	envWave = [][]YmInt{
		env00xx, env00xx, env00xx, env00xx,
		env01xx, env01xx, env01xx, env01xx,
		env1000, env1001, env1010, env1011,
		env1100, env1101, env1110, env1111,
	}

	// Volume table from the original YM2149 implementation, with its /3
	// normalization applied at compile time. Keeping it immutable also makes
	// construction race-free.
	ymVolumeTable = [16]YmInt{
		20, 53, 88, 125, 193, 258, 385, 525,
		753, 1029, 1523, 2077, 3110, 4395, 7073, 10922,
	}

	// Envelope curves are immutable and shared by all chip instances.
	envelopeData = buildEnvelopeData()
)

func buildEnvelopeData() (data [16][2][32]YmU8) {
	for env := 0; env < len(data); env++ {
		pse := envWave[env]
		// Each 32-step phase contains two 16-step segments. The first phase
		// plays once; the second phase repeats until register 13 is written.
		for segment := 0; segment < 4; segment++ {
			a := pse[segment*2]
			b := pse[segment*2+1]
			delta := b - a
			a *= 15
			for i := 0; i < 16; i++ {
				value := a
				a += delta
				if value < 0 {
					value = 0
				} else if value > 15 {
					value = 15
				}
				data[env][segment/2][(segment%2)*16+i] = YmU8(value)
			}
		}
	}
	return data
}

const DC_ADJUST_BUFFERLEN = 512

// DcAdjuster for DC offset adjustment
type DcAdjuster struct {
	buffer [DC_ADJUST_BUFFERLEN]YmInt
	pos    int
	sum    YmInt
}

func NewDcAdjuster() *DcAdjuster {
	return &DcAdjuster{}
}

func (d *DcAdjuster) Reset() {
	for i := range d.buffer {
		d.buffer[i] = 0
	}
	d.pos = 0
	d.sum = 0
}

func (d *DcAdjuster) AddSample(sample YmInt) {
	d.sum -= d.buffer[d.pos]
	d.sum += sample
	d.buffer[d.pos] = sample
	d.pos = (d.pos + 1) & (DC_ADJUST_BUFFERLEN - 1)
}

func (d *DcAdjuster) GetDcLevel() YmInt {
	return d.sum / DC_ADJUST_BUFFERLEN
}

// CYm2149Ex - Extended YM-2149 Emulator
type CYm2149Ex struct {
	bFilter         YmBool
	frameCycle      YmU32
	cyclePerSample  YmU32
	replayFrequency YmInt
	internalClock   YmU32
	registers       [14]YmU8

	// Tone generators
	stepA, stepB, stepC YmU32
	posA, posB, posC    YmU32
	volA, volB, volC    YmInt
	volE                YmInt

	// Mixers
	mixerTA, mixerTB, mixerTC YmU32
	mixerNA, mixerNB, mixerNC YmU32
	pVolA, pVolB, pVolC       *YmInt

	// Noise generator
	noiseStep    YmU32
	noisePos     YmU32
	rndRack      YmU32
	currentNoise YmU32

	// Envelope generator
	envStep  YmU32
	envPos   YmU32
	envPhase YmInt
	envShape YmInt

	// Special effects
	specialEffect   [3]YmSpecialEffect
	effectMask      YmU8
	bSyncBuzzer     YmBool
	syncBuzzerStep  YmU32
	syncBuzzerPhase YmU32

	// Filters
	lowPassFilter [2]int
	dcAdjust      DcAdjuster
}

// NewYm2149Ex creates a new YM2149 emulator
func NewYm2149Ex(masterClock YmU32, prediv YmInt, playRate YmU32) *CYm2149Ex {
	ym := &CYm2149Ex{
		bFilter:         YmTrue,
		internalClock:   masterClock / YmU32(prediv),
		replayFrequency: YmInt(playRate),
	}

	// Set volume voice pointers
	ym.pVolA = &ym.volA
	ym.pVolB = &ym.volB
	ym.pVolC = &ym.volC

	// Reset YM2149
	ym.Reset()

	return ym
}

func (ym *CYm2149Ex) SetClock(clock YmU32) {
	ym.internalClock = clock
}

func (ym *CYm2149Ex) toneStepCompute(rHigh, rLow YmU8) YmU32 {
	per := YmInt(rHigh & 15)
	per = (per << 8) + YmInt(rLow)
	if per <= 5 {
		return 0
	}

	step := YmS64(ym.internalClock)
	step <<= (15 + 16 - 3)
	step /= YmS64(per) * YmS64(ym.replayFrequency)
	return YmU32(step)
}

func (ym *CYm2149Ex) noiseStepCompute(rNoise YmU8) YmU32 {
	per := YmInt(rNoise & 0x1f)
	if per < 3 {
		return 0
	}

	step := YmS64(ym.internalClock)
	step <<= (16 - 1 - 3)
	step /= YmS64(per) * YmS64(ym.replayFrequency)
	return YmU32(step)
}

func (ym *CYm2149Ex) rndCompute() YmU32 {
	rBit := (ym.rndRack & 1) ^ ((ym.rndRack >> 2) & 1)
	ym.rndRack = (ym.rndRack >> 1) | (rBit << 16)
	if rBit != 0 {
		return 0
	}
	return 0xffff
}

func (ym *CYm2149Ex) envStepCompute(rHigh, rLow YmU8) YmU32 {
	per := YmInt(rHigh)
	per = (per << 8) + YmInt(rLow)
	if per < 3 {
		return 0
	}

	step := YmS64(ym.internalClock)
	step <<= (16 + 16 - 9)
	step /= YmS64(per) * YmS64(ym.replayFrequency)
	return YmU32(step)
}

func (ym *CYm2149Ex) Reset() {
	ym.posA, ym.posB, ym.posC, ym.noisePos = 0, 0, 0, 0
	// Clear registers
	for i := range ym.registers {
		ym.registers[i] = 0
	}

	// Write default values
	for i := 0; i < 14; i++ {
		ym.WriteRegister(YmInt(i), 0)
	}
	ym.WriteRegister(7, 0xff)

	ym.currentNoise = 0xffff
	ym.rndRack = 1
	ym.SidStop(0)
	ym.SidStop(1)
	ym.SidStop(2)

	ym.envShape = 0
	ym.envPhase = 0
	ym.envPos = 0

	ym.dcAdjust.Reset()

	for i := range ym.specialEffect {
		ym.specialEffect[i] = YmSpecialEffect{}
	}
	ym.effectMask = 0

	ym.SyncBuzzerStop()

	ym.lowPassFilter[0] = 0
	ym.lowPassFilter[1] = 0
}

func (ym *CYm2149Ex) sidVolumeCompute(voice YmInt, pVol *YmInt) {
	pVoice := &ym.specialEffect[voice]

	if pVoice.Sid {
		if (pVoice.SidPos & (1 << 31)) != 0 {
			ym.WriteRegister(8+voice, YmInt(pVoice.SidVol))
		} else {
			ym.WriteRegister(8+voice, 0)
		}
	} else if pVoice.Drum {
		// DigiDrum playback - exact formula from original
		*pVol = YmInt((YmInt(pVoice.DrumData[pVoice.DrumPos>>DRUM_PREC]) * 255) / 6)

		switch voice {
		case 0:
			ym.pVolA = &ym.volA
			ym.mixerTA = 0xffff
			ym.mixerNA = 0xffff
		case 1:
			ym.pVolB = &ym.volB
			ym.mixerTB = 0xffff
			ym.mixerNB = 0xffff
		case 2:
			ym.pVolC = &ym.volC
			ym.mixerTC = 0xffff
			ym.mixerNC = 0xffff
		}

		pVoice.DrumPos += uint64(pVoice.DrumStep)
		if (pVoice.DrumPos >> DRUM_PREC) >= uint64(pVoice.DrumSize) {
			pVoice.Drum = YmFalse
			if !pVoice.Sid {
				ym.effectMask &^= 1 << voice
			}
		}
	}
}

func (ym *CYm2149Ex) LowPassFilter(in int) int {
	out := (ym.lowPassFilter[0] >> 2) + (ym.lowPassFilter[1] >> 1) + (in >> 2)
	ym.lowPassFilter[0] = ym.lowPassFilter[1]
	ym.lowPassFilter[1] = in
	return out
}

func (ym *CYm2149Ex) nextSample() YmSample {
	// Update noise generator
	if (ym.noisePos & 0xffff0000) != 0 {
		ym.currentNoise ^= ym.rndCompute()
		ym.noisePos &= 0xffff
	}
	bn := ym.currentNoise

	// Update envelope
	ym.volE = ymVolumeTable[envelopeData[ym.envShape][ym.envPhase][ym.envPos>>(32-5)]]

	// Most songs have no per-sample effects. Avoid three function calls and six
	// flag checks on that overwhelmingly common path.
	if ym.effectMask != 0 {
		if ym.effectMask&1 != 0 {
			ym.sidVolumeCompute(0, &ym.volA)
		}
		if ym.effectMask&2 != 0 {
			ym.sidVolumeCompute(1, &ym.volB)
		}
		if ym.effectMask&4 != 0 {
			ym.sidVolumeCompute(2, &ym.volC)
		}
	}

	// Tone+noise+env+DAC for three voices!
	signA := YmU32(YmS32(ym.posA) >> 31)
	btA := (signA | ym.mixerTA) & (bn | ym.mixerNA)
	volA := YmInt(*ym.pVolA) & YmInt(btA)

	signB := YmU32(YmS32(ym.posB) >> 31)
	bt := (signB | ym.mixerTB) & (bn | ym.mixerNB)
	volB := YmInt(*ym.pVolB) & YmInt(bt)

	signC := YmU32(YmS32(ym.posC) >> 31)
	bt = (signC | ym.mixerTC) & (bn | ym.mixerNC)
	volC := YmInt(*ym.pVolC) & YmInt(bt)

	vol := volA + volB + volC

	// Inc
	ym.posA += ym.stepA
	ym.posB += ym.stepB
	ym.posC += ym.stepC
	ym.noisePos += ym.noiseStep
	ym.envPos += ym.envStep

	if ym.envPhase == 0 {
		if ym.envPos < ym.envStep {
			ym.envPhase = 1
		}
	}

	// Sync buzzer is uncommon; keep its accumulator off the normal path.
	if ym.bSyncBuzzer {
		ym.syncBuzzerPhase += ym.syncBuzzerStep
		if (ym.syncBuzzerPhase & (1 << 31)) != 0 {
			ym.envPos = 0
			ym.envPhase = 0
			ym.syncBuzzerPhase &= 0x7fffffff
		}
	}

	// Stopping SID disables volume writes, but its timer keeps running so
	// that enabling it again preserves the phase of the reference player.
	ym.specialEffect[0].SidPos += ym.specialEffect[0].SidStep
	ym.specialEffect[1].SidPos += ym.specialEffect[1].SidStep
	ym.specialEffect[2].SidPos += ym.specialEffect[2].SidStep

	// Normalize process
	ym.dcAdjust.AddSample(vol)
	in := vol - ym.dcAdjust.GetDcLevel()

	if ym.bFilter {
		return YmSample(ym.LowPassFilter(int(in)))
	}
	return YmSample(in)
}

func (ym *CYm2149Ex) ReadRegister(reg YmInt) YmInt {
	if reg >= 0 && reg <= 13 {
		return YmInt(ym.registers[reg])
	}
	return -1
}

func (ym *CYm2149Ex) WriteRegister(reg, data YmInt) {
	switch reg {
	case 0:
		ym.registers[0] = YmU8(data & 255)
		ym.stepA = ym.toneStepCompute(ym.registers[1], ym.registers[0])
		if ym.stepA == 0 {
			ym.posA = 1 << 31
		}

	case 1:
		ym.registers[1] = YmU8(data & 15)
		ym.stepA = ym.toneStepCompute(ym.registers[1], ym.registers[0])
		if ym.stepA == 0 {
			ym.posA = 1 << 31
		}

	case 2:
		ym.registers[2] = YmU8(data & 255)
		ym.stepB = ym.toneStepCompute(ym.registers[3], ym.registers[2])
		if ym.stepB == 0 {
			ym.posB = 1 << 31
		}

	case 3:
		ym.registers[3] = YmU8(data & 15)
		ym.stepB = ym.toneStepCompute(ym.registers[3], ym.registers[2])
		if ym.stepB == 0 {
			ym.posB = 1 << 31
		}

	case 4:
		ym.registers[4] = YmU8(data & 255)
		ym.stepC = ym.toneStepCompute(ym.registers[5], ym.registers[4])
		if ym.stepC == 0 {
			ym.posC = 1 << 31
		}

	case 5:
		ym.registers[5] = YmU8(data & 15)
		ym.stepC = ym.toneStepCompute(ym.registers[5], ym.registers[4])
		if ym.stepC == 0 {
			ym.posC = 1 << 31
		}

	case 6:
		ym.registers[6] = YmU8(data & 0x1f)
		ym.noiseStep = ym.noiseStepCompute(ym.registers[6])
		if ym.noiseStep == 0 {
			ym.noisePos = 0
			ym.currentNoise = 0xffff
		}

	case 7:
		ym.registers[7] = YmU8(data & 255)
		if (data & (1 << 0)) != 0 {
			ym.mixerTA = 0xffff
		} else {
			ym.mixerTA = 0
		}
		if (data & (1 << 1)) != 0 {
			ym.mixerTB = 0xffff
		} else {
			ym.mixerTB = 0
		}
		if (data & (1 << 2)) != 0 {
			ym.mixerTC = 0xffff
		} else {
			ym.mixerTC = 0
		}
		if (data & (1 << 3)) != 0 {
			ym.mixerNA = 0xffff
		} else {
			ym.mixerNA = 0
		}
		if (data & (1 << 4)) != 0 {
			ym.mixerNB = 0xffff
		} else {
			ym.mixerNB = 0
		}
		if (data & (1 << 5)) != 0 {
			ym.mixerNC = 0xffff
		} else {
			ym.mixerNC = 0
		}

	case 8:
		ym.registers[8] = YmU8(data & 31)
		ym.volA = ymVolumeTable[data&15]
		if (data & 0x10) != 0 {
			ym.pVolA = &ym.volE
		} else {
			ym.pVolA = &ym.volA
		}

	case 9:
		ym.registers[9] = YmU8(data & 31)
		ym.volB = ymVolumeTable[data&15]
		if (data & 0x10) != 0 {
			ym.pVolB = &ym.volE
		} else {
			ym.pVolB = &ym.volB
		}

	case 10:
		ym.registers[10] = YmU8(data & 31)
		ym.volC = ymVolumeTable[data&15]
		if (data & 0x10) != 0 {
			ym.pVolC = &ym.volE
		} else {
			ym.pVolC = &ym.volC
		}

	case 11:
		ym.registers[11] = YmU8(data & 255)
		ym.envStep = ym.envStepCompute(ym.registers[12], ym.registers[11])

	case 12:
		ym.registers[12] = YmU8(data & 255)
		ym.envStep = ym.envStepCompute(ym.registers[12], ym.registers[11])

	case 13:
		ym.registers[13] = YmU8(data & 0xf)
		ym.envPos = 0
		ym.envPhase = 0
		ym.envShape = data & 0xf
	}
}

func (ym *CYm2149Ex) Update(pSampleBuffer []YmSample, nbSample YmInt) {
	if nbSample <= 0 {
		return
	}
	if ym.effectMask == 0 && !ym.bSyncBuzzer {
		ym.updateSimple(pSampleBuffer[:int(nbSample)])
		return
	}
	for i := YmInt(0); i < nbSample; i++ {
		pSampleBuffer[i] = ym.nextSample()
	}
}

// updateSimple renders the normal tone/noise/envelope path with the chip state
// held in locals. Register writes only happen between Update calls, so storing
// the accumulators back once per block is equivalent and substantially reduces
// memory traffic in the audio callback.
func (ym *CYm2149Ex) updateSimple(buffer []YmSample) {
	posA, posB, posC := ym.posA, ym.posB, ym.posC
	stepA, stepB, stepC := ym.stepA, ym.stepB, ym.stepC
	mixerTA, mixerTB, mixerTC := ym.mixerTA, ym.mixerTB, ym.mixerTC
	mixerNA, mixerNB, mixerNC := ym.mixerNA, ym.mixerNB, ym.mixerNC

	noisePos, noiseStep := ym.noisePos, ym.noiseStep
	rndRack, currentNoise := ym.rndRack, ym.currentNoise
	envPos, envStep := ym.envPos, ym.envStep
	envPhase, envShape := ym.envPhase, ym.envShape
	envelope := &envelopeData[envShape][envPhase]

	fixedVolA, fixedVolB, fixedVolC := ym.volA, ym.volB, ym.volC
	envA := ym.pVolA == &ym.volE
	envB := ym.pVolB == &ym.volE
	envC := ym.pVolC == &ym.volE
	hasEnvelope := envA || envB || envC

	dcPos, dcSum := ym.dcAdjust.pos, ym.dcAdjust.sum
	filter0, filter1 := ym.lowPassFilter[0], ym.lowPassFilter[1]
	filter := ym.bFilter

	for i := range buffer {
		if noisePos&0xffff0000 != 0 {
			randomBit := (rndRack & 1) ^ ((rndRack >> 2) & 1)
			rndRack = (rndRack >> 1) | (randomBit << 16)
			if randomBit == 0 {
				currentNoise ^= 0xffff
			}
			noisePos &= 0xffff
		}

		volumeA, volumeB, volumeC := fixedVolA, fixedVolB, fixedVolC
		if hasEnvelope {
			envelopeVolume := ymVolumeTable[envelope[envPos>>(32-5)]]
			if envA {
				volumeA = envelopeVolume
			}
			if envB {
				volumeB = envelopeVolume
			}
			if envC {
				volumeC = envelopeVolume
			}
		}

		signA := YmU32(YmS32(posA) >> 31)
		signB := YmU32(YmS32(posB) >> 31)
		signC := YmU32(YmS32(posC) >> 31)
		volume := (volumeA & YmInt((signA|mixerTA)&(currentNoise|mixerNA))) +
			(volumeB & YmInt((signB|mixerTB)&(currentNoise|mixerNB))) +
			(volumeC & YmInt((signC|mixerTC)&(currentNoise|mixerNC)))

		posA += stepA
		posB += stepB
		posC += stepC
		noisePos += noiseStep
		envPos += envStep
		if envPhase == 0 && envPos < envStep {
			envPhase = 1
			envelope = &envelopeData[envShape][envPhase]
		}

		dcSum += volume - ym.dcAdjust.buffer[dcPos]
		ym.dcAdjust.buffer[dcPos] = volume
		dcPos = (dcPos + 1) & (DC_ADJUST_BUFFERLEN - 1)
		// Chip output is unipolar, so the rolling sum cannot be negative. An
		// unsigned shift is exactly equivalent to division by 512 here and avoids
		// signed-division rounding machinery in the innermost loop.
		in := volume - YmInt(YmU32(dcSum)>>9)

		if filter {
			out := (filter0 >> 2) + (filter1 >> 1) + (int(in) >> 2)
			filter0, filter1 = filter1, int(in)
			buffer[i] = YmSample(out)
		} else {
			buffer[i] = YmSample(in)
		}
	}

	ym.posA, ym.posB, ym.posC = posA, posB, posC
	ym.noisePos, ym.rndRack, ym.currentNoise = noisePos, rndRack, currentNoise
	ym.envPos, ym.envPhase = envPos, envPhase
	ym.volE = ymVolumeTable[envelope[envPos>>(32-5)]]
	ym.dcAdjust.pos, ym.dcAdjust.sum = dcPos, dcSum
	ym.lowPassFilter[0], ym.lowPassFilter[1] = filter0, filter1
	for voice := range ym.specialEffect {
		ym.specialEffect[voice].SidPos += ym.specialEffect[voice].SidStep * YmU32(len(buffer))
	}
}

func (ym *CYm2149Ex) DrumStart(voice YmInt, pDrumBuffer []YmU8, drumSize YmU32, drumFreq YmInt) {
	if voice < 0 || voice >= YmInt(len(ym.specialEffect)) || drumFreq <= 0 {
		return
	}
	if uint64(drumSize) > uint64(len(pDrumBuffer)) {
		drumSize = YmU32(len(pDrumBuffer))
	}
	if drumSize > 0 {
		ym.specialEffect[voice].DrumData = pDrumBuffer
		ym.specialEffect[voice].DrumPos = 0
		ym.specialEffect[voice].DrumSize = drumSize
		ym.specialEffect[voice].DrumStep = YmU32((uint64(drumFreq) << DRUM_PREC) / uint64(ym.replayFrequency))
		ym.specialEffect[voice].Drum = YmTrue
		ym.effectMask |= 1 << voice
	}
}

func (ym *CYm2149Ex) DrumStop(voice YmInt) {
	if voice < 0 || voice >= YmInt(len(ym.specialEffect)) {
		return
	}
	ym.specialEffect[voice].Drum = YmFalse
	if !ym.specialEffect[voice].Sid {
		ym.effectMask &^= 1 << voice
	}
}

func (ym *CYm2149Ex) SidStart(voice, timerFreq, vol YmInt) {
	if voice < 0 || voice >= YmInt(len(ym.specialEffect)) || timerFreq < 0 {
		return
	}
	// Preserve the reference integer player's division before multiplication,
	// using an unsigned phase unit instead of a signed shift into bit 31.
	tmp := YmU32(uint64(timerFreq) * ((uint64(1) << 31) / uint64(ym.replayFrequency)))
	ym.specialEffect[voice].SidStep = tmp
	ym.specialEffect[voice].SidVol = vol & 15
	ym.specialEffect[voice].Sid = YmTrue
	ym.effectMask |= 1 << voice
}

func (ym *CYm2149Ex) SidStop(voice YmInt) {
	if voice < 0 || voice >= YmInt(len(ym.specialEffect)) {
		return
	}
	ym.specialEffect[voice].Sid = YmFalse
	if !ym.specialEffect[voice].Drum {
		ym.effectMask &^= 1 << voice
	}
}

func (ym *CYm2149Ex) SyncBuzzerStart(timerFreq, envShape YmInt) {
	if timerFreq < 0 {
		return
	}
	tmp := YmU32(uint64(timerFreq) * ((uint64(1) << 31) / uint64(ym.replayFrequency)))
	ym.envShape = envShape & 15
	ym.syncBuzzerStep = tmp
	ym.syncBuzzerPhase = 0
	ym.bSyncBuzzer = YmTrue
}

func (ym *CYm2149Ex) SyncBuzzerStop() {
	ym.bSyncBuzzer = YmFalse
	ym.syncBuzzerPhase = 0
	ym.syncBuzzerStep = 0
}

func (ym *CYm2149Ex) SetFilter(bFilter YmBool) {
	ym.bFilter = bFilter
}
