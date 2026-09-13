package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// PlaylistItem represents a single item in the playlist
type PlaylistItem struct {
	Path     string `json:"path"`
	Title    string `json:"title"`
	Author   string `json:"author"`
	Duration uint32 `json:"duration"` // in milliseconds
	Comment  string `json:"comment,omitempty"`
	Type     string `json:"type,omitempty"`
}

// Playlist manages a collection of YM files
type Playlist struct {
	Name  string          `json:"name"`
	Items []*PlaylistItem `json:"items"`
}

// NewPlaylist creates a new empty playlist
func NewPlaylist(name string) *Playlist {
	return &Playlist{
		Name:  name,
		Items: make([]*PlaylistItem, 0),
	}
}

// Add adds a new item to the playlist
func (p *Playlist) Add(item *PlaylistItem) {
	p.Items = append(p.Items, item)
}

// Remove removes an item at the specified index
func (p *Playlist) Remove(index int) error {
	if index < 0 || index >= len(p.Items) {
		return fmt.Errorf("index out of range")
	}
	p.Items = append(p.Items[:index], p.Items[index+1:]...)
	return nil
}

// MoveUp moves an item up in the playlist
func (p *Playlist) MoveUp(index int) error {
	if index <= 0 || index >= len(p.Items) {
		return fmt.Errorf("cannot move item up")
	}
	p.Items[index], p.Items[index-1] = p.Items[index-1], p.Items[index]
	return nil
}

// MoveDown moves an item down in the playlist
func (p *Playlist) MoveDown(index int) error {
	if index < 0 || index >= len(p.Items)-1 {
		return fmt.Errorf("cannot move item down")
	}
	p.Items[index], p.Items[index+1] = p.Items[index+1], p.Items[index]
	return nil
}

// Clear removes all items from the playlist
func (p *Playlist) Clear() {
	p.Items = nil
}

// Size returns the number of items in the playlist
func (p *Playlist) Size() int {
	return len(p.Items)
}

// Get returns the item at the specified index
func (p *Playlist) Get(index int) (*PlaylistItem, error) {
	if index < 0 || index >= len(p.Items) {
		return nil, fmt.Errorf("index out of range")
	}
	return p.Items[index], nil
}

// Save saves the playlist to a JSON file
func (p *Playlist) Save(filename string) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

// Load loads a playlist from a JSON file
func LoadPlaylist(filename string) (*Playlist, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var playlist Playlist
	if err := json.Unmarshal(data, &playlist); err != nil {
		return nil, err
	}

	return &playlist, nil
}

// SaveM3U exports the playlist as M3U format
func (p *Playlist) SaveM3U(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write M3U header
	fmt.Fprintln(file, "#EXTM3U")
	fmt.Fprintf(file, "#PLAYLIST:%s\n", p.Name)

	// Write each item
	for _, item := range p.Items {
		duration := int(item.Duration / 1000) // Convert to seconds
		fmt.Fprintf(file, "#EXTINF:%d,%s - %s\n", duration, item.Author, item.Title)
		fmt.Fprintln(file, item.Path)
	}

	return nil
}

// LoadM3U loads a playlist from M3U format
func LoadM3U(filename string) (*Playlist, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	playlist := NewPlaylist(filepath.Base(filename))
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check if file exists and has .ym extension
		if strings.EqualFold(filepath.Ext(line), ".ym") {
			item := &PlaylistItem{
				Path:   filepath.Clean(line),
				Title:  filepath.Base(line),
				Author: "Unknown",
			}
			playlist.Add(item)
		}
	}
	return playlist, scanner.Err()
}

// TotalDuration returns the total duration of all items in milliseconds
func (p *Playlist) TotalDuration() uint32 {
	var total uint32
	for _, item := range p.Items {
		total += item.Duration
	}
	return total
}

// Shuffle randomizes the order of items in the playlist
func (p *Playlist) Shuffle() {
	rand.Shuffle(len(p.Items), func(i, j int) {
		p.Items[i], p.Items[j] = p.Items[j], p.Items[i]
	})
}

// Sort sorts the playlist by a specific field
type SortBy int

const (
	SortByTitle SortBy = iota
	SortByAuthor
	SortByDuration
	SortByPath
)

func (p *Playlist) Sort(by SortBy) {
	var compare func(a, b *PlaylistItem) int
	switch by {
	case SortByTitle:
		compare = func(a, b *PlaylistItem) int { return cmp.Compare(a.Title, b.Title) }
	case SortByAuthor:
		compare = func(a, b *PlaylistItem) int { return cmp.Compare(a.Author, b.Author) }
	case SortByDuration:
		compare = func(a, b *PlaylistItem) int { return cmp.Compare(a.Duration, b.Duration) }
	case SortByPath:
		compare = func(a, b *PlaylistItem) int { return cmp.Compare(a.Path, b.Path) }
	default:
		return
	}
	slices.SortStableFunc(p.Items, compare)
}
