package stsound

// CYmMusic - Main YM music player class
type CYmMusic struct {
	ymChip          *CYm2149Ex
	lastError       string
	songType        YmFileType
	nbFrame         int
	loopFrame       int
	currentFrame    int
	nbDrum          int
	pDrumTab        []DigiDrum
	musicTime       int
	pBigMalloc      []byte
	pDataStream     []byte
	bLoop           YmBool
	fileSize        YmInt
	playerRate      YmInt
	attrib          YmInt
	bMusicOk        YmBool
	bPause          YmBool
	streamInc       int
	innerSamplePos  int
	replayRate      int
	bMusicOver      YmBool

	// Song information
	pSongName    string
	pSongAuthor  string
	pSongComment string
	pSongType    string
	pSongPlayer  string

	// Mix-specific
	nbRepeat            int
	nbMixBlock          int
	pMixBlock           []MixBlock
	mixPos              int
	pBigSampleBuffer    []byte
	pCurrentMixSample   []byte
	currentSampleLength YmU32
	currentPente        YmU32
	currentPos          YmU32

	// Time info
	nbTimeKey                int
	pTimeInfo                []TimeKey
	musicLenInMs             YmU32
	iMusicPosAccurateSample  YmU32
	iMusicPosInMs            YmU32

	// Tracker-specific
	nbVoice                  int
	ymTrackerVoice           [MAX_VOICE]YmTrackerVoice
	ymTrackerNbSampleBefore  int
	ymTrackerVolumeTable     [256 * 64]YmSample
	ymTrackerFreqShift       int
}

// NewYmMusic creates a new YM music player
func NewYmMusic(replayRate int) *CYmMusic {
	if replayRate <= 0 {
		replayRate = 44100
	}

	ym := &CYmMusic{
		replayRate: replayRate,
		ymChip:     NewYm2149Ex(ATARI_CLOCK, 1, YmU32(replayRate)),
		mixPos:     -1,
	}

	ym.SetLoopMode(YmFalse)
	return ym
}

// Public methods
func (ym *CYmMusic) Load(fileName string) error {
	return ym.load(fileName)
}

func (ym *CYmMusic) LoadMemory(data []byte) error {
	return ym.loadMemory(data)
}

func (ym *CYmMusic) UnLoad() {
	ym.unLoad()
}

func (ym *CYmMusic) IsSeekable() YmBool {
	return (ym.attrib & A_TIMECONTROL) != 0
}

func (ym *CYmMusic) Update(pBuffer []YmSample, nbSample int) YmBool {
	if !ym.bMusicOk || ym.bPause || ym.bMusicOver {
		ym.bufferClear(pBuffer, nbSample)
		if ym.bMusicOver {
			return YmFalse
		}
		return YmTrue
	}

	if ym.songType >= YM_MIX1 && ym.songType < YM_MIXMAX {
		ym.stDigitMix(pBuffer, nbSample)
	} else if ym.songType >= YM_TRACKER1 && ym.songType < YM_TRACKERMAX {
		ym.ymTrackerUpdate(pBuffer, nbSample)
	} else {
		pOut := pBuffer
		nbs := nbSample
		vblNbSample := ym.replayRate / int(ym.playerRate)

		for nbs > 0 {
			sampleToCompute := vblNbSample - ym.innerSamplePos
			if sampleToCompute > nbs {
				sampleToCompute = nbs
			}

			ym.innerSamplePos += sampleToCompute
			if ym.innerSamplePos >= vblNbSample {
				ym.player()
				ym.innerSamplePos -= vblNbSample
			}

			if sampleToCompute > 0 {
				ym.ymChip.Update(pOut[:sampleToCompute], YmInt(sampleToCompute))
				pOut = pOut[sampleToCompute:]
			}
			nbs -= sampleToCompute
		}
	}

	return YmTrue
}

func (ym *CYmMusic) GetPos() YmU32 {
	if ym.songType >= YM_MIX1 && ym.songType < YM_MIXMAX {
		return ym.iMusicPosInMs
	} else if ym.nbFrame > 0 && ym.playerRate > 0 {
		return YmU32(ym.currentFrame) * 1000 / YmU32(ym.playerRate)
	}
	return 0
}

func (ym *CYmMusic) GetMusicTime() YmU32 {
	if ym.songType >= YM_MIX1 && ym.songType < YM_MIXMAX {
		return ym.musicLenInMs
	} else if ym.nbFrame > 0 && ym.playerRate > 0 {
		return YmU32(ym.nbFrame) * 1000 / YmU32(ym.playerRate)
	}
	return 0
}

func (ym *CYmMusic) SetMusicTime(time YmU32) YmU32 {
	if !ym.IsSeekable() {
		return 0
	}

	newTime := time
	if ym.songType >= YM_V2 && ym.songType < YM_VMAX {
		if newTime >= ym.GetMusicTime() {
			newTime = 0
		}
		ym.currentFrame = int(newTime * YmU32(ym.playerRate) / 1000)
	} else if ym.songType >= YM_TRACKER1 && ym.songType < YM_TRACKERMAX {
		if newTime >= ym.GetMusicTime() {
			newTime = 0
		}
		ym.currentFrame = int(newTime * YmU32(ym.playerRate) / 1000)
	} else if ym.songType >= YM_MIX1 && ym.songType < YM_MIXMAX {
		ym.setMixTime(time)
	}

	return newTime
}

func (ym *CYmMusic) Restart() {
	ym.SetMusicTime(0)
	ym.bMusicOver = YmFalse
}

func (ym *CYmMusic) Play() {
	ym.play()
}

func (ym *CYmMusic) Pause() {
	ym.pause()
}

func (ym *CYmMusic) Stop() {
	ym.stop()
}

func (ym *CYmMusic) SetLoopMode(bLoop YmBool) {
	ym.bLoop = bLoop
}

func (ym *CYmMusic) GetLastError() string {
	return ym.lastError
}

func (ym *CYmMusic) ReadYmRegister(reg int) int {
	return int(ym.ymChip.ReadRegister(YmInt(reg)))
}

func (ym *CYmMusic) SetLowpassFilter(bActive YmBool) {
	ym.ymChip.SetFilter(bActive)
}

func (ym *CYmMusic) GetMusicInfo() *YmMusicInfo {
	return &YmMusicInfo{
		SongName:      ym.pSongName,
		SongAuthor:    ym.pSongAuthor,
		SongComment:   ym.pSongComment,
		SongType:      ym.pSongType,
		SongPlayer:    ym.pSongPlayer,
		MusicTimeInMs: ym.GetMusicTime(),
	}
}

func (ym *CYmMusic) GetMusicOver() YmBool {
	return ym.bMusicOver
}

// Private methods
func (ym *CYmMusic) setTimeControl(bTime YmBool) {
	if bTime {
		ym.attrib |= A_TIMECONTROL
	} else {
		ym.attrib &= ^A_TIMECONTROL
	}
}

func (ym *CYmMusic) setPlayerRate(rate int) {
	ym.playerRate = YmInt(rate)
}

func (ym *CYmMusic) setAttrib(attrib YmInt) {
	ym.attrib = attrib
}

func (ym *CYmMusic) setLastError(err string) {
	ym.lastError = err
}

func (ym *CYmMusic) bufferClear(pBuffer []YmSample, nbSample int) {
	for i := 0; i < nbSample; i++ {
		pBuffer[i] = 0
	}
}

func (ym *CYmMusic) unLoad() {
	ym.bMusicOk = YmFalse
	ym.bPause = YmTrue
	ym.bMusicOver = YmFalse
	ym.pSongName = ""
	ym.pSongAuthor = ""
	ym.pSongComment = ""
	ym.pSongType = ""
	ym.pSongPlayer = ""
	ym.pBigMalloc = nil
	ym.pDataStream = nil
	ym.pDrumTab = nil
	ym.pBigSampleBuffer = nil
	ym.pMixBlock = nil
	ym.pTimeInfo = nil
	ym.nbDrum = 0
}

func (ym *CYmMusic) stop() {
	ym.bPause = YmTrue
	ym.currentFrame = 0
	ym.iMusicPosInMs = 0
	ym.iMusicPosAccurateSample = 0
	ym.mixPos = -1
}

func (ym *CYmMusic) play() {
	ym.bPause = YmFalse
}

func (ym *CYmMusic) pause() {
	ym.bPause = YmTrue
}

// Player routine
func (ym *CYmMusic) player() {
	if ym.currentFrame < 0 {
		ym.currentFrame = 0
	}

	if ym.currentFrame >= ym.nbFrame {
		if ym.bLoop {
			ym.currentFrame = ym.loopFrame
		} else {
			ym.bMusicOver = YmTrue
			ym.ymChip.Reset()
			return
		}
	}

	ptr := ym.currentFrame * ym.streamInc
	if ptr+ym.streamInc > len(ym.pDataStream) {
		ym.bMusicOver = YmTrue
		return
	}

	data := ym.pDataStream[ptr : ptr+ym.streamInc]

	// Write registers 0-10
	for i := 0; i <= 10; i++ {
		ym.ymChip.WriteRegister(YmInt(i), YmInt(data[i]))
	}

	// Stop all special effects
	ym.ymChip.SidStop(0)
	ym.ymChip.SidStop(1)
	ym.ymChip.SidStop(2)
	ym.ymChip.SyncBuzzerStop()

	// Handle different YM versions
	if ym.songType == YM_V2 {
		// MADMAX specific handling
		if data[13] != 0xff {
			ym.ymChip.WriteRegister(11, YmInt(data[11]))
			ym.ymChip.WriteRegister(12, 0)
			ym.ymChip.WriteRegister(13, 10)
		}
		if (data[10] & 0x80) != 0 {
			sampleNum := data[10] & 0x7f
			if data[12] != 0 {
				sampleFrq := MFP_CLOCK / YmInt(data[12])
				if int(sampleNum) < len(sampleAddress) {
					ym.ymChip.DrumStart(2, sampleAddress[sampleNum], sampleLen[sampleNum], sampleFrq)
				}
			}
		}
	} else if ym.songType >= YM_V3 {
		ym.ymChip.WriteRegister(11, YmInt(data[11]))
		ym.ymChip.WriteRegister(12, YmInt(data[12]))
		if data[13] != 0xff {
			ym.ymChip.WriteRegister(13, YmInt(data[13]))
		}

		if ym.songType >= YM_V5 {
			if ym.songType == YM_V6 {
				ym.readYm6Effect(data, 1, 6, 14)
				ym.readYm6Effect(data, 3, 8, 15)
			} else {
				// YM5 effect decoding
				ym.readYm5Effects(data)
			}
		}
	}

	ym.currentFrame++
}

func (ym *CYmMusic) readYm5Effects(data []byte) {
	// SID Voice
	code := (data[1] >> 4) & 3
	if code != 0 {
		voice := code - 1
		prediv := mfpPrediv[(data[6]>>5)&7]
		prediv *= YmInt(data[14])
		if prediv != 0 {
			tmpFreq := 2457600 / prediv
			ym.ymChip.SidStart(YmInt(voice), tmpFreq, YmInt(data[voice+8]&15))
		}
	}

	// Digi Drum
	code = (data[3] >> 4) & 3
	if code != 0 {
		voice := code - 1
		ndrum := data[8+voice] & 31
		if int(ndrum) < ym.nbDrum {
			prediv := mfpPrediv[(data[8]>>5)&7]
			prediv *= YmInt(data[15])
			if prediv != 0 {
				sampleFrq := MFP_CLOCK / prediv
				ym.ymChip.DrumStart(YmInt(voice),
					ym.pDrumTab[ndrum].Data,
					ym.pDrumTab[ndrum].Size,
					sampleFrq)
			}
		}
	}
}

func (ym *CYmMusic) readYm6Effect(pReg []byte, code, prediv, count int) {
	effectCode := pReg[code] & 0xf0
	predivVal := (pReg[prediv] >> 5) & 7
	countVal := pReg[count]

	if (effectCode & 0x30) != 0 {
		voice := ((effectCode & 0x30) >> 4) - 1

		switch effectCode & 0xc0 {
		case 0x00, 0x80: // SID or Sinus-SID
			p := mfpPrediv[predivVal]
			p *= YmInt(countVal)
			if p != 0 {
				tmpFreq := 2457600 / p
				if (effectCode & 0xc0) == 0x00 {
					ym.ymChip.SidStart(YmInt(voice), tmpFreq, YmInt(pReg[voice+8]&15))
				}
				// TODO: Implement SidSinStart for 0x80
			}

		case 0x40: // DigiDrum
			ndrum := pReg[voice+8] & 31
			if int(ndrum) < ym.nbDrum {
				p := mfpPrediv[predivVal]
				p *= YmInt(countVal)
				if p != 0 {
					tmpFreq := 2457600 / p
					ym.ymChip.DrumStart(YmInt(voice),
						ym.pDrumTab[ndrum].Data,
						ym.pDrumTab[ndrum].Size,
						tmpFreq)
				}
			}

		case 0xc0: // Sync-Buzzer
			p := mfpPrediv[predivVal]
			p *= YmInt(countVal)
			if p != 0 {
				tmpFreq := 2457600 / p
				ym.ymChip.SyncBuzzerStart(tmpFreq, YmInt(pReg[voice+8]&15))
			}
		}
	}
}

// Mix-specific methods
func (ym *CYmMusic) setMixTime(time YmU32) {
	if time > ym.musicLenInMs {
		return
	}

	for i := 0; i < ym.nbTimeKey; i++ {
		var tEnd YmU32
		if i < ym.nbTimeKey-1 {
			tEnd = ym.pTimeInfo[i+1].Time
		} else {
			tEnd = ym.musicLenInMs
		}

		if time >= ym.pTimeInfo[i].Time && time < tEnd {
			ym.mixPos = int(ym.pTimeInfo[i].NBlock)
			ym.pCurrentMixSample = ym.pBigSampleBuffer[ym.pMixBlock[ym.mixPos].SampleStart:]
			ym.currentSampleLength = ym.pMixBlock[ym.mixPos].SampleLength << 12
			ym.currentPente = (YmU32(ym.pMixBlock[ym.mixPos].ReplayFreq) << 12) / YmU32(ym.replayRate)

			len := tEnd - ym.pTimeInfo[i].Time
			t0 := ((time - ym.pTimeInfo[i].Time) * ym.pMixBlock[ym.mixPos].SampleLength) / len

			ym.currentPos = t0 << 12
			ym.nbRepeat = int(ym.pTimeInfo[i].NRepeat)
			break
		}
	}

	ym.iMusicPosInMs = time
	ym.iMusicPosAccurateSample = 0
}

func (ym *CYmMusic) computeTimeInfo() {
	// Compute number of mix blocks
	ym.nbTimeKey = 0
	for i := 0; i < ym.nbMixBlock; i++ {
		if ym.pMixBlock[i].NbRepeat >= 32 {
			ym.pMixBlock[i].NbRepeat = 32
		}
		ym.nbTimeKey += int(ym.pMixBlock[i].NbRepeat)
	}

	// Parse all mixblock keys
	ym.pTimeInfo = make([]TimeKey, ym.nbTimeKey)
	keyIdx := 0
	time := YmU32(0)

	for i := 0; i < ym.nbMixBlock; i++ {
		for j := YmU16(0); j < ym.pMixBlock[i].NbRepeat; j++ {
			ym.pTimeInfo[keyIdx].Time = time
			ym.pTimeInfo[keyIdx].NRepeat = ym.pMixBlock[i].NbRepeat - j
			ym.pTimeInfo[keyIdx].NBlock = YmU16(i)
			keyIdx++

			time += (ym.pMixBlock[i].SampleLength * 1000) / YmU32(ym.pMixBlock[i].ReplayFreq)
		}
	}
	ym.musicLenInMs = time
}

func (ym *CYmMusic) readNextBlockInfo() {
	ym.nbRepeat--
	if ym.nbRepeat <= 0 {
		ym.mixPos++
		if ym.mixPos >= ym.nbMixBlock {
			ym.mixPos = 0
			if !ym.bLoop {
				ym.bMusicOver = YmTrue
			}
			ym.iMusicPosAccurateSample = 0
			ym.iMusicPosInMs = 0
		}
		ym.nbRepeat = int(ym.pMixBlock[ym.mixPos].NbRepeat)
	}

	ym.pCurrentMixSample = ym.pBigSampleBuffer[ym.pMixBlock[ym.mixPos].SampleStart:]
	ym.currentSampleLength = ym.pMixBlock[ym.mixPos].SampleLength << 12
	ym.currentPente = (YmU32(ym.pMixBlock[ym.mixPos].ReplayFreq) << 12) / YmU32(ym.replayRate)
	ym.currentPos &= (1 << 12) - 1
}

func (ym *CYmMusic) stDigitMix(pWrite16 []YmSample, nbs int) {
	if ym.bMusicOver {
		return
	}

	if ym.mixPos == -1 {
		ym.nbRepeat = -1
		ym.readNextBlockInfo()
	}

	ym.iMusicPosAccurateSample += YmU32(nbs * 1000)
	ym.iMusicPosInMs += ym.iMusicPosAccurateSample / YmU32(ym.replayRate)
	ym.iMusicPosAccurateSample %= YmU32(ym.replayRate)

	for i := 0; i < nbs; i++ {
		sa := YmInt(YmSample(ym.pCurrentMixSample[ym.currentPos>>12]) << 8)

		// Linear oversampling
		sb := sa
		if (ym.currentPos >> 12) < ((ym.currentSampleLength >> 12) - 1) {
			sb = YmInt(YmSample(ym.pCurrentMixSample[(ym.currentPos>>12)+1]) << 8)
		}
		frac := ym.currentPos & ((1 << 12) - 1)
		sa += ((sb - sa) * YmInt(frac)) >> 12

		pWrite16[i] = YmSample(sa)

		ym.currentPos += ym.currentPente
		if ym.currentPos >= ym.currentSampleLength {
			ym.readNextBlockInfo()
			if ym.bMusicOver {
				return
			}
		}
	}
}

// Tracker methods
func (ym *CYmMusic) ymTrackerInit(volMaxPercent int) {
	for i := 0; i < MAX_VOICE; i++ {
		ym.ymTrackerVoice[i].Running = YmFalse
	}

	ym.ymTrackerNbSampleBefore = 0

	scale := (256 * volMaxPercent) / (ym.nbVoice * 100)
	idx := 0

	// Build volume table
	for vol := 0; vol < 64; vol++ {
		for s := -128; s < 128; s++ {
			ym.ymTrackerVolumeTable[idx] = YmSample((s * scale * vol) / 64)
			idx++
		}
	}

	// De-interleave if necessary
	ym.ymTrackerDesInterleave()
}

func (ym *CYmMusic) ymTrackerDesInterleave() {
	if (ym.attrib & A_STREAMINTERLEAVED) == 0 {
		return
	}

	size := 4 * ym.nbVoice * ym.nbFrame // sizeof(YmTrackerLine)
	pNewBuffer := make([]byte, size)
	step := 4 * ym.nbVoice

	for n1 := 0; n1 < step; n1++ {
		srcIdx := n1
		dstIdx := n1
		for n2 := 0; n2 < ym.nbFrame; n2++ {
			pNewBuffer[dstIdx] = ym.pDataStream[srcIdx]
			srcIdx++
			dstIdx += step
		}
	}

	copy(ym.pDataStream, pNewBuffer)
	ym.attrib &= ^A_STREAMINTERLEAVED
}

func (ym *CYmMusic) ymTrackerPlayer(pVoice []YmTrackerVoice) {
	lineSize := 4 // sizeof(YmTrackerLine)
	offset := ym.currentFrame * ym.nbVoice * lineSize

	for i := 0; i < ym.nbVoice; i++ {
		idx := offset + i*lineSize
		if idx+3 >= len(ym.pDataStream) {
			break
		}

		line := YmTrackerLine{
			NoteOn:   YmU8(ym.pDataStream[idx]),
			Volume:   YmU8(ym.pDataStream[idx+1]),
			FreqHigh: YmU8(ym.pDataStream[idx+2]),
			FreqLow:  YmU8(ym.pDataStream[idx+3]),
		}

		pVoice[i].SampleFreq = (YmU32(line.FreqHigh) << 8) | YmU32(line.FreqLow)
		if pVoice[i].SampleFreq != 0 {
			pVoice[i].SampleVolume = YmS32(line.Volume & 63)
			pVoice[i].Loop = (line.Volume & 0x40) != 0

			if line.NoteOn != 0xff {
				pVoice[i].Running = YmTrue
				pVoice[i].Sample = ym.pDrumTab[line.NoteOn].Data
				pVoice[i].SampleSize = ym.pDrumTab[line.NoteOn].Size
				pVoice[i].RepLen = ym.pDrumTab[line.NoteOn].RepLen
				pVoice[i].SamplePos = 0
			}
		} else {
			pVoice[i].Running = YmFalse
		}
	}

	ym.currentFrame++
	if ym.currentFrame >= ym.nbFrame {
		if !ym.bLoop {
			ym.bMusicOver = YmTrue
		}
		ym.currentFrame = 0
	}
}

func (ym *CYmMusic) ymTrackerVoiceAdd(pVoice *YmTrackerVoice, pBuffer []YmSample, nbs int) {
	if !pVoice.Running {
		return
	}

	pVolumeTab := ym.ymTrackerVolumeTable[256*(pVoice.SampleVolume&63):]
	samplePos := pVoice.SamplePos

	step := float64(pVoice.SampleFreq<<YMTPREC) * float64(YmU32(1)<<YmU32(ym.ymTrackerFreqShift)) / float64(ym.replayRate)
	sampleInc := YmU32(step)

	sampleEnd := pVoice.SampleSize << YMTPREC
	repLen := pVoice.RepLen << YMTPREC

	for i := 0; i < nbs; i++ {
		if samplePos>>YMTPREC >= YmU32(len(pVoice.Sample)) {
			pVoice.Running = YmFalse
			return
		}

		va := YmInt(pVolumeTab[pVoice.Sample[samplePos>>YMTPREC]])

		// Linear oversampling
		vb := va
		if samplePos < (sampleEnd - (1 << YMTPREC)) {
			nextIdx := (samplePos >> YMTPREC) + 1
			if nextIdx < YmU32(len(pVoice.Sample)) {
				vb = YmInt(pVolumeTab[pVoice.Sample[nextIdx]])
			}
		}

		frac := samplePos & ((1 << YMTPREC) - 1)
		va += ((vb - va) * YmInt(frac)) >> YMTPREC

		pBuffer[i] += YmSample(va)

		samplePos += sampleInc
		if samplePos >= sampleEnd {
			if pVoice.Loop {
				samplePos -= repLen
			} else {
				pVoice.Running = YmFalse
				return
			}
		}
	}

	pVoice.SamplePos = samplePos
}

func (ym *CYmMusic) ymTrackerUpdate(pBuffer []YmSample, nbSample int) {
	// Clear buffer
	for i := 0; i < nbSample; i++ {
		pBuffer[i] = 0
	}

	if ym.bMusicOver {
		return
	}

	remaining := nbSample
	bufIdx := 0

	for remaining > 0 {
		if ym.ymTrackerNbSampleBefore == 0 {
			ym.ymTrackerPlayer(ym.ymTrackerVoice[:])
			if ym.bMusicOver {
				return
			}
			ym.ymTrackerNbSampleBefore = ym.replayRate / int(ym.playerRate)
		}

		nbs := ym.ymTrackerNbSampleBefore
		if nbs > remaining {
			nbs = remaining
		}

		ym.ymTrackerNbSampleBefore -= nbs

		if nbs > 0 {
			for i := 0; i < ym.nbVoice; i++ {
				ym.ymTrackerVoiceAdd(&ym.ymTrackerVoice[i], pBuffer[bufIdx:], nbs)
			}
			bufIdx += nbs
			remaining -= nbs
		}
	}
}