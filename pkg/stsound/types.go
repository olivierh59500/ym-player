package stsound

// YmTypes - Basic types for multi-platform compilation
type (
	YmBool   bool
	YmInt    int32
	YmSample int16
	YmU8     uint8
	YmS8     int8
	YmU16    uint16
	YmS16    int16
	YmU32    uint32
	YmS32    int32
	YmS64    int64
	YmChar   byte
	YmFloat  float32
)

const (
	YmFalse YmBool = false
	YmTrue  YmBool = true
)

// YM file types
type YmFileType int

const (
	YM_V2 YmFileType = iota
	YM_V3
	YM_V4
	YM_V5
	YM_V6
	YM_VMAX

	YM_TRACKER1 YmFileType = 32 + iota
	YM_TRACKER2
	YM_TRACKERMAX

	YM_MIX1 YmFileType = 64 + iota
	YM_MIX2
	YM_MIXMAX
)

// Attributes
const (
	A_STREAMINTERLEAVED = 1 << iota
	A_DRUMSIGNED
	A_DRUM4BITS
	A_TIMECONTROL
	A_LOOPMODE
)

// Constants
const (
	AMSTRAD_CLOCK  = 1000000
	ATARI_CLOCK    = 2000000
	SPECTRUM_CLOCK = 1773400
	MFP_CLOCK      = 2457600
	NOISESIZE      = 16384
	DRUM_PREC      = 15
	SIDSINPOWER    = 0.7
	YMTPREC        = 16
	MAX_VOICE      = 8
	MAX_DIGIDRUM   = 128
)

// Voice constants
const (
	VOICE_A = 0
	VOICE_B = 1
	VOICE_C = 2
)

// YmMusicInfo represents music information
type YmMusicInfo struct {
	SongName      string
	SongAuthor    string
	SongComment   string
	SongType      string
	SongPlayer    string
	MusicTimeInMs YmU32
}

// MixBlock represents a mix block
type MixBlock struct {
	SampleStart  YmU32
	SampleLength YmU32
	NbRepeat     YmU16
	ReplayFreq   YmU16
}

// DigiDrum represents a digital drum sample
type DigiDrum struct {
	Size   YmU32
	Data   []YmU8
	RepLen YmU32
}

// YmTrackerVoice represents a tracker voice
type YmTrackerVoice struct {
	Sample       []YmU8
	SampleSize   YmU32
	SamplePos    YmU32
	RepLen       YmU32
	SampleVolume YmS32
	SampleFreq   YmU32
	Loop         YmBool
	Running      YmBool
}

// YmTrackerLine represents a tracker line
type YmTrackerLine struct {
	NoteOn   YmU8
	Volume   YmU8
	FreqHigh YmU8
	FreqLow  YmU8
}

// YmSpecialEffect represents special effects
type YmSpecialEffect struct {
	Drum     YmBool
	DrumSize YmU32
	DrumData []YmU8
	DrumPos  YmU32
	DrumStep YmU32

	Sid     YmBool
	SidPos  YmU32
	SidStep YmU32
	SidVol  YmInt
}

// TimeKey for time information
type TimeKey struct {
	Time    YmU32
	NRepeat YmU16
	NBlock  YmU16
}
