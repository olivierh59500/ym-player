package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadM3UParsesLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playlist.m3u")
	data := "#EXTM3U\n#EXTINF:1,Author - One\none.ym\r\nfolder/two.YM\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	playlist, err := LoadM3U(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(playlist.Items) != 2 || playlist.Items[0].Path != "one.ym" || playlist.Items[1].Path != filepath.Join("folder", "two.YM") {
		t.Fatalf("unexpected items: %+v", playlist.Items)
	}
}

func TestPlaylistSort(t *testing.T) {
	playlist := &Playlist{Items: []*PlaylistItem{
		{Title: "C", Author: "A", Duration: 2},
		{Title: "A", Author: "B", Duration: 3},
		{Title: "B", Author: "A", Duration: 1},
	}}
	playlist.Sort(SortByAuthor)
	if playlist.Items[0].Title != "C" || playlist.Items[1].Title != "B" || playlist.Items[2].Title != "A" {
		t.Fatalf("sort is not stable: %+v", playlist.Items)
	}
	playlist.Sort(SortByDuration)
	for i, duration := range []uint32{1, 2, 3} {
		if playlist.Items[i].Duration != duration {
			t.Fatalf("item %d has duration %d, want %d", i, playlist.Items[i].Duration, duration)
		}
	}
}

func BenchmarkPlaylistSort1000(b *testing.B) {
	items := make([]*PlaylistItem, 1000)
	for i := range items {
		items[i] = &PlaylistItem{Title: fmt.Sprintf("title-%04d", len(items)-i)}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		playlist := &Playlist{Items: append([]*PlaylistItem(nil), items...)}
		playlist.Sort(SortByTitle)
	}
}
