package stsound

import (
	"bytes"
	"fmt"
)

// decodeTracker follows the YMT1/YMT2 layout in Arnaud Carre's original ST-Sound
// Ymload.cpp. These are sampled tracker streams, not YM register dumps.
func (ym *CYmMusic) decodeTracker(id YmU32) error {
	r := ymDataReader{data: ym.pBigMalloc, offset: 4}
	sig, err := r.take(8)
	if err != nil || string(sig) != "LeOnArD!" {
		return fmt.Errorf("invalid YMT signature")
	}
	voices, err := r.uint16()
	if err != nil {
		return err
	}
	rate, err := r.uint16()
	if err != nil {
		return err
	}
	frames, err := r.uint32()
	if err != nil {
		return err
	}
	loop, err := r.uint32()
	if err != nil {
		return err
	}
	drums, err := r.uint16()
	if err != nil {
		return err
	}
	flags, err := r.uint32()
	if err != nil {
		return err
	}
	if voices == 0 || voices > MAX_VOICE || rate == 0 || int(rate) > ym.replayRate || frames == 0 {
		return fmt.Errorf("invalid YMT dimensions or replay rate")
	}
	if loop >= frames {
		loop = 0
	}
	ym.nbVoice, ym.playerRate, ym.nbFrame, ym.loopFrame, ym.nbDrum = int(voices), YmInt(rate), int(frames), int(loop), int(drums)
	ym.attrib = YmInt(flags & 0x0fffffff)
	ym.songType = YM_TRACKER1
	ym.ymTrackerFreqShift = 0
	ym.pSongType = "YM-T1"
	if id == e_YMT2 {
		ym.songType = YM_TRACKER2
		ym.ymTrackerFreqShift = int(flags >> 28)
		ym.pSongType = "YM-T2"
	}
	if ym.pSongName, err = r.ntString(); err != nil {
		return err
	}
	if ym.pSongAuthor, err = r.ntString(); err != nil {
		return err
	}
	if ym.pSongComment, err = r.ntString(); err != nil {
		return err
	}
	// Sample indices occupy one byte, but the file can contain a larger bank
	// with unused entries, or no samples at all in a silent pattern stream.
	drumHeaderSize := uint64(2)
	if id == e_YMT2 {
		drumHeaderSize = 6
	}
	if uint64(drums)*drumHeaderSize > uint64(len(r.remaining())) {
		return fmt.Errorf("truncated YMT sample bank")
	}
	ym.pDrumTab = make([]DigiDrum, drums)
	for i := range ym.pDrumTab {
		size, err := r.uint16()
		if err != nil {
			return err
		}
		repeat := size
		if id == e_YMT2 {
			repeat, err = r.uint16()
			if err != nil {
				return err
			}
			if _, err = r.uint16(); err != nil {
				return err
			}
			repeat = min(repeat, size)
		}
		data, err := r.take(int(size))
		if err != nil {
			return err
		}
		drum := DigiDrum{Size: YmU32(size), RepLen: YmU32(repeat), Data: make([]YmU8, len(data))}
		for j, v := range data {
			drum.Data[j] = YmU8(v)
		}
		ym.pDrumTab[i] = drum
	}
	length := uint64(frames) * uint64(voices) * 4
	if length > uint64(len(r.remaining())) {
		return fmt.Errorf("truncated YMT pattern stream")
	}
	data, err := r.take(int(length))
	if err != nil {
		return err
	}
	ym.pDataStream = bytes.Clone(data)
	ym.streamInc = 4 * int(voices)
	ym.ymTrackerInit(100)
	// Validate references after deinterleaving, before the realtime callback.
	for i := 0; i < len(ym.pDataStream); i += 4 {
		line := ym.pDataStream[i : i+4]
		if line[2] != 0 || line[3] != 0 {
			if line[0] != 255 && int(line[0]) >= len(ym.pDrumTab) {
				return fmt.Errorf("YMT sample index out of range")
			}
		}
	}
	ym.currentFrame = 0
	ym.pSongPlayer = "Universal Tracker"
	ym.setTimeControl(YmTrue)
	return nil
}

// decodeMix loads the sampled MIX1 files distributed with ST-Sound's digi music
// collection. Samples are owned so unsigned conversion never mutates caller data.
func (ym *CYmMusic) decodeMix() error {
	r := ymDataReader{data: ym.pBigMalloc, offset: 4}
	sig, err := r.take(8)
	if err != nil || string(sig) != "LeOnArD!" {
		return fmt.Errorf("invalid MIX1 signature")
	}
	flags, err := r.uint32()
	if err != nil {
		return err
	}
	size, err := r.uint32()
	if err != nil {
		return err
	}
	blocks, err := r.uint32()
	if err != nil {
		return err
	}
	if size == 0 || blocks == 0 || uint64(blocks)*12 > uint64(len(r.remaining())) {
		return fmt.Errorf("invalid MIX1 sample or block count")
	}
	ym.songType = YM_MIX1
	ym.nbMixBlock = int(blocks)
	ym.pMixBlock = make([]MixBlock, blocks)
	for i := range ym.pMixBlock {
		start, err := r.uint32()
		if err != nil {
			return err
		}
		length, err := r.uint32()
		if err != nil {
			return err
		}
		repeat, err := r.uint16()
		if err != nil {
			return err
		}
		rate, err := r.uint16()
		if err != nil {
			return err
		}
		if length == 0 || uint64(start)+uint64(length) > uint64(size) || repeat == 0 || rate == 0 {
			return fmt.Errorf("invalid MIX1 block %d", i)
		}
		if uint64(rate)<<12 < uint64(ym.replayRate) {
			return fmt.Errorf("MIX1 block %d rate is too low for the output sample rate", i)
		}
		ym.pMixBlock[i] = MixBlock{SampleStart: start, SampleLength: length, NbRepeat: repeat, ReplayFreq: rate}
	}
	if ym.pSongName, err = r.ntString(); err != nil {
		return err
	}
	if ym.pSongAuthor, err = r.ntString(); err != nil {
		return err
	}
	if ym.pSongComment, err = r.ntString(); err != nil {
		return err
	}
	data, err := r.take(int(size))
	if err != nil {
		return err
	}
	ym.pBigSampleBuffer = bytes.Clone(data)
	if flags&1 == 0 {
		for i := range ym.pBigSampleBuffer {
			ym.pBigSampleBuffer[i] ^= 0x80
		}
	}
	ym.attrib = A_DRUMSIGNED | A_TIMECONTROL
	ym.computeTimeInfo()
	ym.mixPos = -1
	ym.currentPente = 0
	ym.currentPos = 0
	ym.pSongType = "MIX1"
	ym.pSongPlayer = "Digi-Mix driver"
	return nil
}
