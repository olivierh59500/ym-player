package lzh

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func levelZeroArchive(method string, payload []byte, originalSize uint32) []byte {
	const headerSize = 22
	data := make([]byte, headerSize+2+len(payload))
	data[0] = headerSize
	copy(data[2:7], method)
	binary.LittleEndian.PutUint32(data[7:11], uint32(len(payload)))
	binary.LittleEndian.PutUint32(data[11:15], originalSize)
	copy(data[headerSize+2:], payload)
	return data
}

type testBits struct {
	data []byte
	bits int
}

func (w *testBits) put(value, count int) {
	for bit := count - 1; bit >= 0; bit-- {
		if w.bits%8 == 0 {
			w.data = append(w.data, 0)
		}
		w.data[w.bits/8] |= byte((value>>bit)&1) << (7 - w.bits%8)
		w.bits++
	}
}

// singletonBlock encodes a complete block using the three single-symbol
// Huffman-table forms. No external compressor or binary fixture is needed.
func singletonBlock(symbol, count int) []byte {
	var bits testBits
	bits.put(count, 16)
	bits.put(0, TBIT)
	bits.put(0, TBIT)
	bits.put(0, CBIT)
	bits.put(symbol, CBIT)
	bits.put(0, PBIT)
	bits.put(0, PBIT)
	return bits.data
}

func TestDecompressLevelZeroLH4AndLH5(t *testing.T) {
	for _, method := range []string{"-lh4-", "-lh5-"} {
		t.Run(method, func(t *testing.T) {
			want := bytes.Repeat([]byte{'A'}, 9000)
			data := levelZeroArchive(method, singletonBlock('A', len(want)), uint32(len(want)))
			got, err := Decompress(data)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatal("single-symbol blocks changed across dictionary boundaries")
			}
		})
	}
}

func TestLegacyLH5HeaderMatchesSTSound(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "testdata", "Whittaker David", "Xenon II.ym"))
	if err != nil {
		t.Fatal(err)
	}
	if int(data[0])+2 == 24+int(data[21]) {
		t.Fatal("fixture no longer contains its original inconsistent header size")
	}
	got, err := Decompress(data)
	if err != nil {
		t.Fatal(err)
	}
	// Produced independently by ST-Sound's original C++ LH5 depacker.
	const want = "ef8b7930fd766e95abad3e68151d732bd44239654026b3d2ec6b1732b4086ede"
	if hash := fmt.Sprintf("%x", sha256.Sum256(got)); hash != want {
		t.Fatalf("reference decompression differs: got %s, want %s", hash, want)
	}
}

func TestDecompressRejectsInvalidHeadersAndHuffmanStreams(t *testing.T) {
	valid := levelZeroArchive("-lh5-", singletonBlock('A', 1), 1)
	level := bytes.Clone(valid)
	level[20] = 1
	filename := bytes.Clone(valid)
	filename[21] = 255
	shortHeader := bytes.Clone(valid)
	shortHeader[0] = 1
	emptyBlock := levelZeroArchive("-lh5-", singletonBlock('A', 0), 1)
	invalidSymbol := levelZeroArchive("-lh5-", singletonBlock(511, 1), 1)
	var oversubscribed testBits
	oversubscribed.put(1, 16)
	oversubscribed.put(3, TBIT)
	for i := 0; i < 3; i++ {
		oversubscribed.put(1, 3)
	}
	oversubscribed.put(0, 2)
	var incomplete testBits
	incomplete.put(1, 16)
	incomplete.put(1, TBIT)
	incomplete.put(2, 3)
	for name, data := range map[string][]byte{
		"nonzero header level": level,
		"truncated filename":   filename,
		"invalid header size":  shortHeader,
		"missing payload":      levelZeroArchive("-lh5-", nil, 16),
		"forged enormous size": levelZeroArchive("-lh5-", nil, ^uint32(0)),
		"empty block":          emptyBlock,
		"invalid character":    invalidSymbol,
		"oversubscribed tree":  levelZeroArchive("-lh5-", oversubscribed.data, 1),
		"incomplete tree":      levelZeroArchive("-lh5-", incomplete.data, 1),
		"truncated bitstream":  valid[:len(valid)-2],
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decompress(data); err == nil {
				t.Fatal("malformed archive was accepted")
			}
		})
	}
}

func TestDecompressRejectsTruncatedRealPayload(t *testing.T) {
	data := compressedFixture(t)
	payloadStart := 24 + int(data[21])
	for _, end := range []int{payloadStart, payloadStart + 16, len(data) / 2, len(data) - 16} {
		if _, err := Decompress(data[:end]); err == nil {
			t.Fatalf("accepted truncated archive at %d/%d bytes", end, len(data))
		}
	}
}

func FuzzDecompress(f *testing.F) {
	f.Add(levelZeroArchive("-lh5-", singletonBlock('A', 1), 1))
	f.Add(levelZeroArchive("-lh0-", []byte("YM3!"), 4))
	f.Fuzz(func(t *testing.T, data []byte) {
		// Bound requested output for fuzzing resource use, independently of
		// input validity. The public decoder still accepts larger real songs.
		for start := 0; start+15 <= len(data); start++ {
			if IsLZHCompressed(data[start:]) {
				if binary.LittleEndian.Uint32(data[start+11:start+15]) > 1<<20 {
					return
				}
				break
			}
		}
		_, _ = Decompress(data)
	})
}
