package stsound

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/olivierh59500/ym-player/pkg/lzh"
)

// MFP chip predivisor
var mfpPrediv = []YmInt{0, 4, 10, 16, 50, 64, 100, 200}

// LZH Header structure
type LzhHeader struct {
	Size       YmU8
	Sum        YmU8
	ID         [5]byte
	Packed     YmU32
	Original   YmU32
	Reserved   [5]YmU8
	Level      YmU8
	NameLength YmU8
}

// File ID constants - ces valeurs sont en big-endian
const (
	e_YM2a = YmU32(0x594D3221) // 'YM2!'
	e_YM3a = YmU32(0x594D3321) // 'YM3!'
	e_YM3b = YmU32(0x594D3362) // 'YM3b'
	e_YM4a = YmU32(0x594D3421) // 'YM4!'
	e_YM5a = YmU32(0x594D3521) // 'YM5!'
	e_YM6a = YmU32(0x594D3621) // 'YM6!'
	e_MIX1 = YmU32(0x4D495831) // 'MIX1'
	e_YMT1 = YmU32(0x594D5431) // 'YMT1'
	e_YMT2 = YmU32(0x594D5432) // 'YMT2'
)

// Fonctions de lecture avec endianness explicite
func readBigEndian32(data []byte) YmU32 {
	if len(data) < 4 {
		return 0
	}
	return YmU32(data[0])<<24 | YmU32(data[1])<<16 | YmU32(data[2])<<8 | YmU32(data[3])
}

func readLittleEndian32(data []byte) YmU32 {
	if len(data) < 4 {
		return 0
	}
	return YmU32(data[0]) | YmU32(data[1])<<8 | YmU32(data[2])<<16 | YmU32(data[3])<<24
}

type ymDataReader struct {
	data   []byte
	offset int
}

func (r *ymDataReader) take(size int) ([]byte, error) {
	if size < 0 || size > len(r.data)-r.offset {
		return nil, errors.New("unexpected end of YM data")
	}
	data := r.data[r.offset : r.offset+size]
	r.offset += size
	return data, nil
}

func (r *ymDataReader) uint16() (YmU16, error) {
	data, err := r.take(2)
	if err != nil {
		return 0, err
	}
	return YmU16(data[0])<<8 | YmU16(data[1]), nil
}

func (r *ymDataReader) uint32() (YmU32, error) {
	data, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return readBigEndian32(data), nil
}

func (r *ymDataReader) ntString() (string, error) {
	data := r.data[r.offset:]
	end := bytes.IndexByte(data, 0)
	if end < 0 {
		return "", errors.New("unterminated YM metadata")
	}
	r.offset += end + 1
	return string(data[:end]), nil
}

func (r *ymDataReader) remaining() []byte {
	return r.data[r.offset:]
}

func signeSample(data []YmU8) {
	for i := range data {
		data[i] ^= 0x80
	}
}

// Load functions
func (ym *CYmMusic) load(fileName string) error {
	ym.stop()
	ym.unLoad()

	// Read file
	data, err := os.ReadFile(fileName)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	ym.pBigMalloc = data
	ym.fileSize = YmInt(len(data))

	// Depack if necessary
	depackedData, err := ym.depackFile()
	if err != nil {
		return err
	}
	ym.pBigMalloc = depackedData
	ym.fileSize = YmInt(len(depackedData))

	// Decode YM format
	if err := ym.ymDecode(false); err != nil {
		return err
	}

	ym.ymChip.Reset()
	ym.bMusicOk = YmTrue
	ym.bPause = YmFalse
	return nil
}

func (ym *CYmMusic) loadMemory(data []byte) error {
	ym.stop()
	ym.unLoad()

	// Decode directly from the caller's slice. deInterleave creates the owned
	// playback stream for interleaved files; the uncommon planar case is cloned
	// explicitly before returning.
	compressed := lzh.IsLZHCompressed(data)
	ym.pBigMalloc = data
	ym.fileSize = YmInt(len(data))

	// Depack if necessary
	depackedData, err := ym.depackFile()
	if err != nil {
		return err
	}
	ym.pBigMalloc = depackedData
	ym.fileSize = YmInt(len(depackedData))

	// Decode YM format
	if err := ym.ymDecode(!compressed); err != nil {
		return err
	}

	ym.ymChip.Reset()
	ym.bMusicOk = YmTrue
	ym.bPause = YmFalse
	return nil
}

func (ym *CYmMusic) depackFile() ([]byte, error) {
	if len(ym.pBigMalloc) < 22 {
		return ym.pBigMalloc, nil
	}

	// Check for LH5 compression
	if lzh.IsLZHCompressed(ym.pBigMalloc) {
		decompressed, err := lzh.Decompress(ym.pBigMalloc)
		if err != nil {
			return nil, fmt.Errorf("LZH decompression failed: %w", err)
		}
		return decompressed, nil
	}

	// Not compressed, return as-is
	return ym.pBigMalloc, nil
}

func (ym *CYmMusic) deInterleave(cloneStream bool) error {
	if (ym.attrib & A_STREAMINTERLEAVED) == 0 {
		if cloneStream {
			ym.pDataStream = bytes.Clone(ym.pDataStream)
			ym.pBigMalloc = ym.pDataStream
		}
		return nil
	}

	tmpBuff := make([]byte, ym.nbFrame*ym.streamInc)

	// YM2/3 and YM5/6 have fixed register counts. Unrolling those two layouts
	// avoids a multiplication and loop branch for every register in every frame.
	n := ym.nbFrame
	src := ym.pDataStream[:n*ym.streamInc]
	switch ym.streamInc {
	case 16:
		for frame, dst := 0, 0; frame < n; frame, dst = frame+1, dst+16 {
			tmpBuff[dst+0] = src[frame]
			tmpBuff[dst+1] = src[n+frame]
			tmpBuff[dst+2] = src[2*n+frame]
			tmpBuff[dst+3] = src[3*n+frame]
			tmpBuff[dst+4] = src[4*n+frame]
			tmpBuff[dst+5] = src[5*n+frame]
			tmpBuff[dst+6] = src[6*n+frame]
			tmpBuff[dst+7] = src[7*n+frame]
			tmpBuff[dst+8] = src[8*n+frame]
			tmpBuff[dst+9] = src[9*n+frame]
			tmpBuff[dst+10] = src[10*n+frame]
			tmpBuff[dst+11] = src[11*n+frame]
			tmpBuff[dst+12] = src[12*n+frame]
			tmpBuff[dst+13] = src[13*n+frame]
			tmpBuff[dst+14] = src[14*n+frame]
			tmpBuff[dst+15] = src[15*n+frame]
		}
	case 14:
		for frame, dst := 0, 0; frame < n; frame, dst = frame+1, dst+14 {
			tmpBuff[dst+0] = src[frame]
			tmpBuff[dst+1] = src[n+frame]
			tmpBuff[dst+2] = src[2*n+frame]
			tmpBuff[dst+3] = src[3*n+frame]
			tmpBuff[dst+4] = src[4*n+frame]
			tmpBuff[dst+5] = src[5*n+frame]
			tmpBuff[dst+6] = src[6*n+frame]
			tmpBuff[dst+7] = src[7*n+frame]
			tmpBuff[dst+8] = src[8*n+frame]
			tmpBuff[dst+9] = src[9*n+frame]
			tmpBuff[dst+10] = src[10*n+frame]
			tmpBuff[dst+11] = src[11*n+frame]
			tmpBuff[dst+12] = src[12*n+frame]
			tmpBuff[dst+13] = src[13*n+frame]
		}
	default:
		for frame := 0; frame < n; frame++ {
			dst := frame * ym.streamInc
			for register := 0; register < ym.streamInc; register++ {
				tmpBuff[dst+register] = src[register*n+frame]
			}
		}
	}

	ym.pBigMalloc = tmpBuff
	ym.pDataStream = tmpBuff
	ym.attrib &= ^A_STREAMINTERLEAVED

	return nil
}

func (ym *CYmMusic) ymDecode(cloneStream bool) error {
	if len(ym.pBigMalloc) < 4 {
		return errors.New("file too small")
	}

	// Read file ID in big-endian (YM files use big-endian for headers)
	id := readBigEndian32(ym.pBigMalloc[:4])

	switch id {
	case e_YM2a: // YM2!
		if len(ym.pBigMalloc) < 4+14 {
			return errors.New("truncated YM2 stream")
		}
		ym.songType = YM_V2
		ym.nbFrame = int((ym.fileSize - 4) / 14)
		ym.loopFrame = 0
		ym.ymChip.SetClock(ATARI_CLOCK)
		ym.setPlayerRate(50)
		ym.pDataStream = ym.pBigMalloc[4:]
		ym.streamInc = 14
		ym.nbDrum = 0
		ym.setAttrib(A_STREAMINTERLEAVED | A_TIMECONTROL)
		ym.pSongName = "Unknown"
		ym.pSongAuthor = "Unknown"
		ym.pSongComment = "Converted by Leonard."
		ym.pSongType = "YM 2"
		ym.pSongPlayer = "YM-Chip driver"

	case e_YM3a: // YM3!
		if len(ym.pBigMalloc) < 4+14 {
			return errors.New("truncated YM3 stream")
		}
		ym.songType = YM_V3
		ym.nbFrame = int((ym.fileSize - 4) / 14)
		ym.loopFrame = 0
		ym.ymChip.SetClock(ATARI_CLOCK)
		ym.setPlayerRate(50)
		ym.pDataStream = ym.pBigMalloc[4:]
		ym.streamInc = 14
		ym.nbDrum = 0
		ym.setAttrib(A_STREAMINTERLEAVED | A_TIMECONTROL)
		ym.pSongName = "Unknown"
		ym.pSongAuthor = "Unknown"
		ym.pSongComment = ""
		ym.pSongType = "YM 3"
		ym.pSongPlayer = "YM-Chip driver"

	case e_YM3b: // YM3b
		if len(ym.pBigMalloc) < 4+14+4 {
			return errors.New("truncated YM3b stream")
		}
		// YM3b stocke le loop frame à la fin en little-endian
		pUD := ym.pBigMalloc[ym.fileSize-4:]
		ym.songType = YM_V3
		ym.nbFrame = int((ym.fileSize - 4) / 14)
		ym.loopFrame = int(readLittleEndian32(pUD))
		ym.ymChip.SetClock(ATARI_CLOCK)
		ym.setPlayerRate(50)
		ym.pDataStream = ym.pBigMalloc[4:]
		ym.streamInc = 14
		ym.nbDrum = 0
		ym.setAttrib(A_STREAMINTERLEAVED | A_TIMECONTROL)
		ym.pSongName = "Unknown"
		ym.pSongAuthor = "Unknown"
		ym.pSongComment = ""
		ym.pSongType = "YM 3b (loop)"
		ym.pSongPlayer = "YM-Chip driver"

	case e_YM5a, e_YM6a: // YM5! or YM6!
		if len(ym.pBigMalloc) < 12 || !bytes.Equal(ym.pBigMalloc[4:12], []byte("LeOnArD!")) {
			return errors.New("not a valid YM format")
		}

		reader := ymDataReader{data: ym.pBigMalloc[12:]}
		nbFrame, err := reader.uint32()
		if err != nil {
			return fmt.Errorf("invalid YM5/6 header: %w", err)
		}
		attributes, err := reader.uint32()
		if err != nil {
			return fmt.Errorf("invalid YM5/6 header: %w", err)
		}
		nbDrum, err := reader.uint16()
		if err != nil {
			return fmt.Errorf("invalid YM5/6 header: %w", err)
		}
		clock, err := reader.uint32()
		if err != nil {
			return fmt.Errorf("invalid YM5/6 header: %w", err)
		}
		playerRate, err := reader.uint16()
		if err != nil {
			return fmt.Errorf("invalid YM5/6 header: %w", err)
		}
		loopFrame, err := reader.uint32()
		if err != nil {
			return fmt.Errorf("invalid YM5/6 header: %w", err)
		}
		skip, err := reader.uint16()
		if err != nil {
			return fmt.Errorf("invalid YM5/6 header: %w", err)
		}
		if _, err := reader.take(int(skip)); err != nil {
			return fmt.Errorf("invalid YM5/6 extra data: %w", err)
		}

		ym.nbFrame = int(nbFrame)
		ym.setAttrib(YmInt(attributes) | A_TIMECONTROL)
		ym.nbDrum = int(nbDrum)
		ym.ymChip.SetClock(clock)
		ym.setPlayerRate(int(playerRate))
		ym.loopFrame = int(loopFrame)
		if ym.nbDrum > MAX_DIGIDRUM {
			return fmt.Errorf("too many digidrums: %d", ym.nbDrum)
		}

		// Load drums if present
		if ym.nbDrum > 0 {
			ym.pDrumTab = make([]DigiDrum, ym.nbDrum)
			for i := 0; i < ym.nbDrum; i++ {
				drumSize, err := reader.uint32()
				if err != nil {
					return fmt.Errorf("invalid digidrum %d: %w", i, err)
				}
				drumData, err := reader.take(int(drumSize))
				if err != nil {
					return fmt.Errorf("invalid digidrum %d: %w", i, err)
				}
				ym.pDrumTab[i].Size = drumSize
				if drumSize > 0 {
					ym.pDrumTab[i].Data = make([]YmU8, len(drumData))
					for j, sample := range drumData {
						ym.pDrumTab[i].Data[j] = YmU8(sample)
					}

					// Traiter les drums 4 bits si nécessaire
					if (ym.attrib & A_DRUM4BITS) != 0 {
						for j := range ym.pDrumTab[i].Data {
							ym.pDrumTab[i].Data[j] = YmU8(ymVolumeTable[ym.pDrumTab[i].Data[j]&15] >> 7)
						}
					}
				}
			}
			ym.attrib &= ^A_DRUM4BITS
		}

		ym.pSongName, err = reader.ntString()
		if err != nil {
			return fmt.Errorf("invalid song name: %w", err)
		}
		ym.pSongAuthor, err = reader.ntString()
		if err != nil {
			return fmt.Errorf("invalid song author: %w", err)
		}
		ym.pSongComment, err = reader.ntString()
		if err != nil {
			return fmt.Errorf("invalid song comment: %w", err)
		}

		if id == e_YM6a {
			ym.songType = YM_V6
			ym.pSongType = "YM 6"
		} else {
			ym.songType = YM_V5
			ym.pSongType = "YM 5"
		}

		// The buffer already points into pBigMalloc. Interleaved streams are
		// copied once into their playback layout by deInterleave below.
		ym.pDataStream = reader.remaining()
		ym.streamInc = 16
		ym.pSongPlayer = "YM-Chip driver"

	case e_YM4a: // YM4!
		// YM4 est similaire à YM3 mais sans support pour l'instant
		return errors.New("YM4 format not yet supported")

	default:
		// Vérifier si c'est peut-être un format avec un ID différent
		// Essayer de lire comme string pour debug
		idStr := string(ym.pBigMalloc[:4])
		return fmt.Errorf("unknown YM format: %s (0x%08X)", idStr, id)
	}

	if ym.nbFrame <= 0 {
		return errors.New("YM stream contains no frames")
	}
	if ym.playerRate <= 0 {
		return errors.New("YM player rate must be positive")
	}
	streamSize := uint64(ym.nbFrame) * uint64(ym.streamInc)
	if streamSize > uint64(len(ym.pDataStream)) {
		return fmt.Errorf("truncated YM register stream: got %d bytes, need %d", len(ym.pDataStream), streamSize)
	}

	return ym.deInterleave(cloneStream)
}
