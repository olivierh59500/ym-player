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

	// Volume table - valeurs originales du YM2149
	ymVolumeTable = []YmInt{
		62, 161, 265, 377, 580, 774, 1155, 1575,
		2260, 3088, 4570, 6233, 9330, 13187, 21220, 32767,
	}

	volumeTableInitialized = false
)

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
	envData  [16][2][32]YmU8  // 16 shapes, 2 phases (pas 4!), 32 steps

	// Special effects
	specialEffect [3]YmSpecialEffect
	bSyncBuzzer   YmBool
	syncBuzzerStep YmU32
	syncBuzzerPhase YmU32

	// Filters
	lowPassFilter [2]int
	dcAdjust      *DcAdjuster
}

// NewYm2149Ex creates a new YM2149 emulator
func NewYm2149Ex(masterClock YmU32, prediv YmInt, playRate YmU32) *CYm2149Ex {
	ym := &CYm2149Ex{
		bFilter:         YmTrue,
		internalClock:   masterClock / YmU32(prediv),
		replayFrequency: YmInt(playRate),
		dcAdjust:        NewDcAdjuster(),
	}

	// Restaurer la division par 6 comme dans l'original
	if !volumeTableInitialized && ymVolumeTable[15] == 32767 {
		volumeTableInitialized = true
		for i := range ymVolumeTable {
			ymVolumeTable[i] = (ymVolumeTable[i] * 2) / 6
		}
	}

	// Build envelope shapes
	ym.initEnvelopeData()

	// Set volume voice pointers
	ym.pVolA = &ym.volA
	ym.pVolB = &ym.volB
	ym.pVolC = &ym.volC

	// Reset YM2149
	ym.Reset()

	return ym
}

func (ym *CYm2149Ex) initEnvelopeData() {
	// Exactement comme dans le C++
	for env := 0; env < 16; env++ {
		pse := envWave[env]
		pEnv := 0
		for phase := 0; phase < 4; phase++ {
			a := pse[phase*2]
			b := pse[phase*2+1]
			d := b - a
			a *= 15
			for i := 0; i < 16; i++ {
				val := a
				a += d
				if val < 0 {
					val = 0
				} else if val > 15 {
					val = 15
				}
				// Phase 0 et 1 seulement (le C++ utilise 2 phases avec 32 positions)
				if phase < 2 {
					ym.envData[env][phase][pEnv] = YmU8(val)
					pEnv++
				}
			}
			if phase == 1 {
				pEnv = 0  // Reset pour la phase suivante
			}
		}
	}
}

func (ym *CYm2149Ex) SetClock(clock YmU32) {
	ym.internalClock = clock
}

func (ym *CYm2149Ex) toneStepCompute(rHigh, rLow YmU8) YmU32 {
	per := YmInt(rHigh&15)
	per = (per << 8) + YmInt(rLow)
	if per <= 5 {
		return 0
	}

	step := YmS64(ym.internalClock)
	step <<= (15 + 16 - 3)
	step /= YmS64(per * ym.replayFrequency)
	return YmU32(step)
}

func (ym *CYm2149Ex) noiseStepCompute(rNoise YmU8) YmU32 {
	per := YmInt(rNoise & 0x1f)
	if per < 3 {
		return 0
	}

	step := YmS64(ym.internalClock)
	step <<= (16 - 1 - 3)
	step /= YmS64(per * ym.replayFrequency)
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
	step /= YmS64(per * ym.replayFrequency)
	return YmU32(step)
}

func (ym *CYm2149Ex) Reset() {
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

		pVoice.DrumPos += pVoice.DrumStep
		if (pVoice.DrumPos >> DRUM_PREC) >= pVoice.DrumSize {
			pVoice.Drum = YmFalse
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
	ym.volE = ymVolumeTable[ym.envData[ym.envShape][ym.envPhase][ym.envPos>>(32-5)]]

	// Update special effects
	ym.sidVolumeCompute(0, &ym.volA)
	ym.sidVolumeCompute(1, &ym.volB)
	ym.sidVolumeCompute(2, &ym.volC)

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

	// Sync buzzer
	ym.syncBuzzerPhase += ym.syncBuzzerStep
	if (ym.syncBuzzerPhase & (1 << 31)) != 0 {
		ym.envPos = 0
		ym.envPhase = 0
		ym.syncBuzzerPhase &= 0x7fffffff
	}

	// Update SID positions
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
	for i := YmInt(0); i < nbSample; i++ {
		pSampleBuffer[i] = ym.nextSample()
	}
}

func (ym *CYm2149Ex) DrumStart(voice YmInt, pDrumBuffer []YmU8, drumSize YmU32, drumFreq YmInt) {
	if len(pDrumBuffer) > 0 && drumSize > 0 {
		ym.specialEffect[voice].DrumData = pDrumBuffer
		ym.specialEffect[voice].DrumPos = 0
		ym.specialEffect[voice].DrumSize = drumSize
		ym.specialEffect[voice].DrumStep = YmU32((drumFreq << DRUM_PREC) / ym.replayFrequency)
		ym.specialEffect[voice].Drum = YmTrue
	}
}

func (ym *CYm2149Ex) DrumStop(voice YmInt) {
	ym.specialEffect[voice].Drum = YmFalse
}

func (ym *CYm2149Ex) SidStart(voice, timerFreq, vol YmInt) {
	// Version integer only
	tmp := YmU32(timerFreq) * (YmU32(1) << 31) / YmU32(ym.replayFrequency)
	ym.specialEffect[voice].SidStep = tmp
	ym.specialEffect[voice].SidVol = vol & 15
	ym.specialEffect[voice].Sid = YmTrue
}

func (ym *CYm2149Ex) SidStop(voice YmInt) {
	ym.specialEffect[voice].Sid = YmFalse
}

func (ym *CYm2149Ex) SyncBuzzerStart(timerFreq, envShape YmInt) {
	tmp := YmU32(timerFreq) * (YmU32(1) << 31) / YmU32(ym.replayFrequency)
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