package stsound

import (
	"bytes"
	//	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"

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

func readBigEndian16(data []byte) YmU16 {
	if len(data) < 2 {
		return 0
	}
	return YmU16(data[0])<<8 | YmU16(data[1])
}

func readLittleEndian32(data []byte) YmU32 {
	if len(data) < 4 {
		return 0
	}
	return YmU32(data[0]) | YmU32(data[1])<<8 | YmU32(data[2])<<16 | YmU32(data[3])<<24
}

func readLittleEndian16(data []byte) YmU16 {
	if len(data) < 2 {
		return 0
	}
	return YmU16(data[0]) | YmU16(data[1])<<8
}

// Lecture depuis un buffer avec big-endian (Motorola byte order)
func readMotorolaDword(buf *bytes.Buffer) YmU32 {
	data := make([]byte, 4)
	buf.Read(data)
	return readBigEndian32(data)
}

func readMotorolaWord(buf *bytes.Buffer) YmU16 {
	data := make([]byte, 2)
	buf.Read(data)
	return readBigEndian16(data)
}

func readNtString(buf *bytes.Buffer) string {
	var result []byte
	for {
		b, err := buf.ReadByte()
		if err != nil || b == 0 {
			break
		}
		result = append(result, b)
	}
	return string(result)
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
	depackedData, err := ym.depackFile(YmU32(len(data)))
	if err != nil {
		return err
	}
	ym.pBigMalloc = depackedData

	// Decode YM format
	if err := ym.ymDecode(); err != nil {
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

	// Copy data
	ym.pBigMalloc = make([]byte, len(data))
	copy(ym.pBigMalloc, data)
	ym.fileSize = YmInt(len(data))

	// Depack if necessary
	depackedData, err := ym.depackFile(YmU32(len(data)))
	if err != nil {
		return err
	}
	ym.pBigMalloc = depackedData

	// Decode YM format
	if err := ym.ymDecode(); err != nil {
		return err
	}

	ym.ymChip.Reset()
	ym.bMusicOk = YmTrue
	ym.bPause = YmFalse
	return nil
}

func (ym *CYmMusic) depackFile(checkOriginalSize YmU32) ([]byte, error) {
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

func (ym *CYmMusic) deInterleave() error {
	if (ym.attrib & A_STREAMINTERLEAVED) == 0 {
		return nil
	}

	tmpBuff := make([]byte, ym.nbFrame*ym.streamInc)

	// YM format stores data in a specific interleaved format
	// We need to de-interleave it properly
	for voice := 0; voice < ym.streamInc; voice++ {
		srcOffset := voice * ym.nbFrame
		for frame := 0; frame < ym.nbFrame; frame++ {
			dstOffset := frame*ym.streamInc + voice
			tmpBuff[dstOffset] = ym.pDataStream[srcOffset+frame]
		}
	}

	ym.pBigMalloc = tmpBuff
	ym.pDataStream = tmpBuff
	ym.attrib &= ^A_STREAMINTERLEAVED

	return nil
}

func (ym *CYmMusic) ymDecode() error {
	if len(ym.pBigMalloc) < 4 {
		return errors.New("file too small")
	}

	// Read file ID in big-endian (YM files use big-endian for headers)
	id := readBigEndian32(ym.pBigMalloc[:4])

	switch id {
	case e_YM2a: // YM2!
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
		// Vérifier la signature LeOnArD!
		if !strings.HasPrefix(string(ym.pBigMalloc[4:12]), "LeOnArD!") {
			return errors.New("not a valid YM format")
		}

		// YM5/6 utilise big-endian pour l'en-tête
		buf := bytes.NewBuffer(ym.pBigMalloc[12:])

		ym.nbFrame = int(readMotorolaDword(buf))
		ym.setAttrib(YmInt(readMotorolaDword(buf)) | A_TIMECONTROL)
		ym.nbDrum = int(readMotorolaWord(buf))
		ym.ymChip.SetClock(readMotorolaDword(buf))
		ym.setPlayerRate(int(readMotorolaWord(buf)))
		ym.loopFrame = int(readMotorolaDword(buf))
		skip := readMotorolaWord(buf)

		// Skip additional data
		buf.Next(int(skip))

		// Load drums if present
		if ym.nbDrum > 0 {
			ym.pDrumTab = make([]DigiDrum, ym.nbDrum)
			for i := 0; i < ym.nbDrum; i++ {
				// Drum size en big-endian
				ym.pDrumTab[i].Size = readMotorolaDword(buf)
				if ym.pDrumTab[i].Size > 0 {
					// Allouer et lire les données
					tmpData := make([]byte, ym.pDrumTab[i].Size)
					buf.Read(tmpData)

					// Convertir en YmU8
					ym.pDrumTab[i].Data = make([]YmU8, len(tmpData))
					for j := range tmpData {
						ym.pDrumTab[i].Data[j] = YmU8(tmpData[j])
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

		// Lire les métadonnées (null-terminated strings)
		ym.pSongName = readNtString(buf)
		ym.pSongAuthor = readNtString(buf)
		ym.pSongComment = readNtString(buf)

		if id == e_YM6a {
			ym.songType = YM_V6
			ym.pSongType = "YM 6"
		} else {
			ym.songType = YM_V5
			ym.pSongType = "YM 5"
		}

		// Les données sont le reste du buffer
		remaining := buf.Len()
		ym.pDataStream = make([]byte, remaining)
		buf.Read(ym.pDataStream)
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

	return ym.deInterleave()
}
