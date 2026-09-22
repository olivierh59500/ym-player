package lzh

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func compressedFixture(t testing.TB) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "testdata", "Hubbard Robb", "Goldrunner.ym"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDecompressRegression(t *testing.T) {
	data := compressedFixture(t)
	got, err := Decompress(data)
	if err != nil {
		t.Fatal(err)
	}
	if string(got[:4]) != "YM5!" {
		t.Fatalf("unexpected signature %q", got[:4])
	}
	if len(got) != 150770 {
		t.Fatalf("decompressed size changed: got %d, want 150770", len(got))
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(got))
	const wantHash = "5e39ee8ded34f0df52c2d1f5650b84f0b4cded25144e937eb4a81ce8c96699a2"
	if hash != wantHash {
		t.Fatalf("decompressed hash changed: got %s, want %s", hash, wantHash)
	}
}

func TestDecompressRejectsMalformedDataWithoutPanic(t *testing.T) {
	data := []byte{0, 0, '-', 'l', 'h', '5', '-'}
	if _, err := Decompress(data); err == nil {
		t.Fatal("expected malformed archive to be rejected")
	}
}

func TestDecompressLH0(t *testing.T) {
	payload := []byte("YM3!uncompressed")
	const headerSize = 22
	archive := make([]byte, headerSize+2+len(payload))
	archive[0] = headerSize
	copy(archive[2:7], "-lh0-")
	binary.LittleEndian.PutUint32(archive[7:11], uint32(len(payload)))
	binary.LittleEndian.PutUint32(archive[11:15], uint32(len(payload)))
	copy(archive[headerSize+2:], payload)

	got, err := Decompress(archive)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("got %q, want %q", got, payload)
	}
}

func BenchmarkDecompress(b *testing.B) {
	data := compressedFixture(b)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Decompress(data); err != nil {
			b.Fatal(err)
		}
	}
}
