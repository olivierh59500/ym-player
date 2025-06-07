package main

import (
	"fmt"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/olivierh59500/ym-player/pkg/audio"
	"github.com/olivierh59500/ym-player/pkg/stsound"
)

type YMPlayerGUI struct {
	app    fyne.App
	window fyne.Window

	// Player
	player      *stsound.StSound
	audioOutput audio.Output
	buffer      []int16
	playing     bool
	paused      bool
	mutex       sync.Mutex

	// Playlist
	playlist       *Playlist
	currentIndex   int
	playlistWidget *widget.List
	shuffle        bool
	repeatMode     RepeatMode

	// UI Elements
	titleLabel   *widget.Label
	authorLabel  *widget.Label
	commentLabel *widget.Label
	typeLabel    *widget.Label
	timeLabel    *widget.Label
	progressBar  *widget.ProgressBar
	volumeSlider *widget.Slider
	playButton   *widget.Button
	pauseButton  *widget.Button
	stopButton   *widget.Button
	prevButton   *widget.Button
	nextButton   *widget.Button
	loopCheck    *widget.Check
	lowpassCheck *widget.Check
	shuffleCheck *widget.Check
	repeatButton *widget.Button
	cpuLabel     *widget.Label

	// Playlist UI
	addButton      *widget.Button
	removeButton   *widget.Button
	clearButton    *widget.Button
	moveUpButton   *widget.Button
	moveDownButton *widget.Button
	playlistLabel  *widget.Label

	// File info
	currentFile string
	duration    uint32
	position    uint32

	// Settings
	volume     float64
	sampleRate int
	bufferSize int
	loop       bool
	lowpass    bool

	// Update ticker
	ticker *time.Ticker
	done   chan bool

	// UI update values (for thread safety)
	uiProgress float64
	uiTimeText string
	uiStatus   string
	uiMutex    sync.Mutex
}

// RepeatMode defines playlist repeat behavior
type RepeatMode int

const (
	RepeatNone RepeatMode = iota
	RepeatOne
	RepeatAll
)

// Custom theme with better colors for dark/light mode
type modernTheme struct{}

func (m modernTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if variant == theme.VariantLight {
		switch name {
		case theme.ColorNameBackground:
			return color.NRGBA{250, 250, 250, 255}
		case theme.ColorNameButton:
			return color.NRGBA{240, 240, 240, 255}
		case theme.ColorNameForeground:
			return color.NRGBA{20, 20, 20, 255}
		case theme.ColorNamePrimary:
			return color.NRGBA{33, 150, 243, 255}
		case theme.ColorNameHover:
			return color.NRGBA{230, 230, 230, 255}
		case theme.ColorNameInputBackground:
			return color.NRGBA{255, 255, 255, 255}
		case theme.ColorNamePlaceHolder:
			return color.NRGBA{160, 160, 160, 255}
		case theme.ColorNameScrollBar:
			return color.NRGBA{200, 200, 200, 255}
		case theme.ColorNameShadow:
			return color.NRGBA{0, 0, 0, 66}
		}
	} else {
		// Dark theme colors
		switch name {
		case theme.ColorNameBackground:
			return color.NRGBA{30, 30, 30, 255}
		case theme.ColorNameButton:
			return color.NRGBA{50, 50, 50, 255}
		case theme.ColorNameForeground:
			return color.NRGBA{240, 240, 240, 255}
		case theme.ColorNamePrimary:
			return color.NRGBA{64, 196, 255, 255}
		case theme.ColorNameHover:
			return color.NRGBA{70, 70, 70, 255}
		case theme.ColorNameInputBackground:
			return color.NRGBA{40, 40, 40, 255}
		case theme.ColorNamePlaceHolder:
			return color.NRGBA{120, 120, 120, 255}
		case theme.ColorNameScrollBar:
			return color.NRGBA{80, 80, 80, 255}
		case theme.ColorNameShadow:
			return color.NRGBA{0, 0, 0, 128}
		}
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (m modernTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (m modernTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (m modernTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInlineIcon:
		return 24
	case theme.SizeNameScrollBar:
		return 16
	}
	return theme.DefaultTheme().Size(name)
}

func NewYMPlayerGUI() *YMPlayerGUI {
	p := &YMPlayerGUI{
		app:          app.New(),
		volume:       1.0,
		sampleRate:   44100,
		bufferSize:   2048,
		loop:         false,
		lowpass:      true,
		done:         make(chan bool),
		playlist:     NewPlaylist("Default"),
		currentIndex: -1,
		repeatMode:   RepeatNone,
	}

	// Set modern theme
	p.app.Settings().SetTheme(&modernTheme{})
	p.createUI()

	return p
}

func (p *YMPlayerGUI) createUI() {
	p.window = p.app.NewWindow("YM Player - ST-Sound")
	p.window.Resize(fyne.NewSize(900, 650))

	// Create menu
	fileMenu := fyne.NewMenu("File",
		fyne.NewMenuItem("Add Files...", p.addFiles),
		fyne.NewMenuItem("Add Folder...", p.addFolder),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Save Playlist...", p.savePlaylist),
		fyne.NewMenuItem("Load Playlist...", p.loadPlaylist),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Export Current to WAV...", p.exportWAV),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", p.app.Quit),
	)

	playlistMenu := fyne.NewMenu("Playlist",
		fyne.NewMenuItem("Clear All", p.clearPlaylist),
		fyne.NewMenuItem("Sort by Title", func() { p.sortPlaylist(SortByTitle) }),
		fyne.NewMenuItem("Sort by Author", func() { p.sortPlaylist(SortByAuthor) }),
		fyne.NewMenuItem("Sort by Duration", func() { p.sortPlaylist(SortByDuration) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Shuffle", p.shufflePlaylist),
	)

	helpMenu := fyne.NewMenu("Help",
		fyne.NewMenuItem("About", p.showAbout),
	)

	mainMenu := fyne.NewMainMenu(fileMenu, playlistMenu, helpMenu)
	p.window.SetMainMenu(mainMenu)

	// Create main content
	mainContent := p.createMainContent()
	playlistContent := p.createPlaylistContent()

	// Create split container
	split := container.NewHSplit(mainContent, playlistContent)
	split.SetOffset(0.6) // 60% for main, 40% for playlist

	p.window.SetContent(split)
	p.window.SetOnClosed(p.cleanup)

	// Start update ticker
	p.startUpdateTicker()
}

func (p *YMPlayerGUI) createMainContent() fyne.CanvasObject {
	// Create info panel with card styling
	p.titleLabel = widget.NewLabel("No file loaded")
	p.titleLabel.TextStyle = fyne.TextStyle{Bold: true}

	p.authorLabel = widget.NewLabel("")
	p.commentLabel = widget.NewLabel("")
	p.typeLabel = widget.NewLabel("")

	infoContent := container.NewVBox(
		p.titleLabel,
		p.authorLabel,
		p.commentLabel,
		p.typeLabel,
	)

	infoCard := widget.NewCard("Now Playing", "", infoContent)

	// Create time display
	p.timeLabel = widget.NewLabel("00:00 / 00:00")
	p.timeLabel.Alignment = fyne.TextAlignCenter
	p.progressBar = widget.NewProgressBar()

	timeContainer := container.NewVBox(
		p.progressBar,
		p.timeLabel,
	)

	// Create control buttons with better styling
	p.prevButton = widget.NewButtonWithIcon("", theme.MediaSkipPreviousIcon(), p.playPrevious)
	p.playButton = widget.NewButtonWithIcon("", theme.MediaPlayIcon(), p.play)
	p.pauseButton = widget.NewButtonWithIcon("", theme.MediaPauseIcon(), p.pause)
	p.stopButton = widget.NewButtonWithIcon("", theme.MediaStopIcon(), p.stop)
	p.nextButton = widget.NewButtonWithIcon("", theme.MediaSkipNextIcon(), p.playNext)

	p.playButton.Disable()
	p.pauseButton.Disable()
	p.stopButton.Disable()
	p.prevButton.Disable()
	p.nextButton.Disable()

	buttonContainer := container.NewHBox(
		layout.NewSpacer(),
		p.prevButton,
		p.playButton,
		p.pauseButton,
		p.stopButton,
		p.nextButton,
		layout.NewSpacer(),
	)

	// Create volume control
	p.volumeSlider = widget.NewSlider(0, 2)
	p.volumeSlider.Value = 1.0
	p.volumeSlider.Step = 0.01 // Allow 1% increments
	volumeLabel := widget.NewLabel("100%")

	p.volumeSlider.OnChanged = func(value float64) {
		p.mutex.Lock()
		p.volume = value
		p.mutex.Unlock()
		volumeLabel.SetText(fmt.Sprintf("%.0f%%", value*100))
	}

	volumeIcon := widget.NewIcon(theme.VolumeUpIcon())
	volumeContainer := container.NewBorder(
		nil, nil,
		container.NewHBox(volumeIcon, widget.NewLabel("Volume:")),
		volumeLabel,
		p.volumeSlider,
	)

	// Create options
	p.loopCheck = widget.NewCheck("Loop Track", func(checked bool) {
		p.mutex.Lock()
		p.loop = checked
		if p.player != nil {
			p.player.SetLoopMode(checked)
		}
		p.mutex.Unlock()
	})

	p.lowpassCheck = widget.NewCheck("Low-pass Filter", func(checked bool) {
		p.mutex.Lock()
		p.lowpass = checked
		if p.player != nil {
			p.player.SetLowpassFilter(checked)
		}
		p.mutex.Unlock()
	})
	p.lowpassCheck.SetChecked(true)

	p.shuffleCheck = widget.NewCheck("Shuffle", func(checked bool) {
		p.shuffle = checked
	})

	p.repeatButton = widget.NewButton("Repeat: Off", p.toggleRepeatMode)

	optionsContainer := container.NewHBox(
		p.loopCheck,
		p.lowpassCheck,
		widget.NewSeparator(),
		p.shuffleCheck,
		p.repeatButton,
	)

	// Create tip card
	tipCard := widget.NewCard("", "", widget.NewLabelWithStyle(
		"💡 Tip: Use the Add button or menu to add YM files to your playlist",
		fyne.TextAlignCenter,
		fyne.TextStyle{Italic: true},
	))

	// Create status bar
	p.cpuLabel = widget.NewLabel("Ready")
	statusBar := container.NewBorder(
		widget.NewSeparator(), nil, nil, p.cpuLabel, nil,
	)

	// Create main layout
	content := container.NewVBox(
		infoCard,
		widget.NewSeparator(),
		timeContainer,
		buttonContainer,
		widget.NewSeparator(),
		volumeContainer,
		optionsContainer,
		layout.NewSpacer(),
		tipCard,
		statusBar,
	)

	return container.NewPadded(content)
}

func (p *YMPlayerGUI) createPlaylistContent() fyne.CanvasObject {
	// Create playlist label
	p.playlistLabel = widget.NewLabel("Playlist (0 items)")
	p.playlistLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Create playlist widget
	p.playlistWidget = widget.NewList(
		func() int {
			return p.playlist.Size()
		},
		func() fyne.CanvasObject {
			title := widget.NewLabel("")
			title.Truncation = fyne.TextTruncateEllipsis
			duration := widget.NewLabel("")
			return container.NewBorder(nil, nil, nil, duration, title)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			box := item.(*fyne.Container)
			titleLabel := box.Objects[0].(*widget.Label)
			durationLabel := box.Objects[1].(*widget.Label)

			playlistItem, _ := p.playlist.Get(id)
			if playlistItem != nil {
				// Format: "Title - Author"
				text := fmt.Sprintf("%s - %s", playlistItem.Title, playlistItem.Author)
				titleLabel.SetText(text)
				durationLabel.SetText(formatTime(playlistItem.Duration))

				// Highlight current item
				if id == p.currentIndex {
					titleLabel.TextStyle = fyne.TextStyle{Bold: true}
				} else {
					titleLabel.TextStyle = fyne.TextStyle{}
				}
			}
		},
	)

	// Double-click to play
	p.playlistWidget.OnSelected = func(id widget.ListItemID) {
		p.playFromIndex(id)
	}

	// Create playlist buttons
	p.addButton = widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), p.addFiles)
	p.removeButton = widget.NewButtonWithIcon("Remove", theme.ContentRemoveIcon(), p.removeSelected)
	p.clearButton = widget.NewButtonWithIcon("Clear", theme.DeleteIcon(), p.clearPlaylist)
	p.moveUpButton = widget.NewButtonWithIcon("", theme.MoveUpIcon(), p.moveSelectedUp)
	p.moveDownButton = widget.NewButtonWithIcon("", theme.MoveDownIcon(), p.moveSelectedDown)

	p.removeButton.Disable()
	p.moveUpButton.Disable()
	p.moveDownButton.Disable()

	buttonBar := container.NewHBox(
		p.addButton,
		p.removeButton,
		p.clearButton,
		layout.NewSpacer(),
		p.moveUpButton,
		p.moveDownButton,
	)

	// Create playlist container
	playlistCard := widget.NewCard("", "", container.NewBorder(
		container.NewVBox(
			p.playlistLabel,
			widget.NewSeparator(),
		),
		buttonBar,
		nil, nil,
		container.NewScroll(p.playlistWidget),
	))

	return playlistCard
}

func (p *YMPlayerGUI) startUpdateTicker() {
	p.ticker = time.NewTicker(100 * time.Millisecond)

	// Start background update goroutine
	go func() {
		for {
			select {
			case <-p.ticker.C:
				p.prepareUIUpdate()
			case <-p.done:
				return
			}
		}
	}()

	// Start main thread UI updater
	go func() {
		for {
			select {
			case <-time.After(100 * time.Millisecond):
				if p.window == nil {
					return
				}
				p.applyUIUpdate()
			case <-p.done:
				return
			}
		}
	}()
}

func (p *YMPlayerGUI) prepareUIUpdate() {
	// This runs in background thread - only read values
	p.mutex.Lock()
	playing := p.playing
	paused := p.paused
	hasPlayer := p.player != nil
	position := p.position
	duration := p.duration

	if hasPlayer && playing && !paused {
		// Update position while locked
		p.position = p.player.GetPos()
		position = p.position
	}
	p.mutex.Unlock()

	// Prepare UI values
	p.uiMutex.Lock()
	defer p.uiMutex.Unlock()

	// Calculate progress
	if duration > 0 {
		p.uiProgress = float64(position) / float64(duration)
		posStr := formatTime(position)
		durStr := formatTime(duration)
		p.uiTimeText = fmt.Sprintf("%s / %s", posStr, durStr)
	} else {
		p.uiProgress = 0
		p.uiTimeText = "00:00 / 00:00"
	}

	// Status
	if playing && !paused {
		p.uiStatus = "Playing"
	} else if paused {
		p.uiStatus = "Paused"
	} else {
		p.uiStatus = "Ready"
	}
}

func (p *YMPlayerGUI) applyUIUpdate() {
	// This should run in main thread
	if p.window == nil || p.progressBar == nil {
		return
	}

	p.uiMutex.Lock()
	progress := p.uiProgress
	timeText := p.uiTimeText
	status := p.uiStatus
	p.uiMutex.Unlock()

	// Apply updates - these should be safe in main thread
	if p.progressBar != nil {
		p.progressBar.SetValue(progress)
	}

	if p.timeLabel != nil {
		p.timeLabel.SetText(timeText)
	}

	if p.cpuLabel != nil {
		p.cpuLabel.SetText(status)
	}
}

// Remove the old updateUI function

// All other methods remain the same as in the previous version...
func (p *YMPlayerGUI) addFiles() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		reader.Close()

		p.addFileToPlaylist(reader.URI().Path())
	}, p.window)
}

func (p *YMPlayerGUI) addFolder() {
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil || uri == nil {
			return
		}

		// List all files in folder
		files, err := uri.List()
		if err != nil {
			dialog.ShowError(err, p.window)
			return
		}

		// Add all YM files
		added := 0
		for _, file := range files {
			if strings.HasSuffix(strings.ToLower(file.Name()), ".ym") ||
				strings.HasSuffix(strings.ToLower(file.Name()), ".lzh") {
				p.addFileToPlaylist(file.Path())
				added++
			}
		}

		if added > 0 {
			dialog.ShowInformation("Files Added",
				fmt.Sprintf("Added %d YM files to playlist", added), p.window)
		}
	}, p.window)
}

func (p *YMPlayerGUI) addFileToPlaylist(filePath string) {
	// Create temporary player to get file info
	tempPlayer := stsound.CreateWithRate(p.sampleRate)
	defer tempPlayer.Destroy()

	// Try to load file
	if err := tempPlayer.Load(filePath); err != nil {
		log.Printf("Failed to load %s: %v", filePath, err)
		return
	}

	// Get file info
	info := tempPlayer.GetInfo()

	// Create playlist item
	item := &PlaylistItem{
		Path:     filePath,
		Title:    info.SongName,
		Author:   info.SongAuthor,
		Duration: uint32(info.MusicTimeInMs),
		Comment:  info.SongComment,
		Type:     info.SongType,
	}

	// Clean up empty titles/authors
	if item.Title == "" || item.Title == "Unknown" {
		item.Title = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}
	if item.Author == "" {
		item.Author = "Unknown"
	}

	// Add to playlist
	p.playlist.Add(item)
	p.updatePlaylistLabel()
	p.playlistWidget.Refresh()

	// Enable play button if this is the first item
	if p.playlist.Size() == 1 {
		p.playButton.Enable()
		p.currentIndex = 0
	}
}

func (p *YMPlayerGUI) loadYMData(filename string, data []byte) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Stop current playback
	if p.playing {
		p.playing = false
		if p.audioOutput != nil {
			p.audioOutput.Close()
			p.audioOutput = nil
		}
	}

	// Destroy old player
	if p.player != nil {
		p.player.Destroy()
	}

	// Create new player
	p.player = stsound.CreateWithRate(p.sampleRate)
	p.buffer = make([]int16, p.bufferSize)

	// Load YM data
	if err := p.player.LoadMemory(data); err != nil {
		dialog.ShowError(err, p.window)
		p.player.Destroy()
		p.player = nil
		return
	}

	// Update UI with song info
	info := p.player.GetInfo()

	p.titleLabel.SetText(info.SongName)
	p.authorLabel.SetText("by " + info.SongAuthor)
	if info.SongComment != "" {
		p.commentLabel.SetText(info.SongComment)
	} else {
		p.commentLabel.SetText("")
	}
	p.typeLabel.SetText(info.SongType + " • " + info.SongPlayer)

	p.currentFile = filename
	p.duration = uint32(info.MusicTimeInMs)
	p.position = 0

	// Set options
	p.player.SetLoopMode(p.loop || p.repeatMode == RepeatOne)
	p.player.SetLowpassFilter(p.lowpass)

	// Update progress
	p.progressBar.SetValue(0)

	// Update time label
	p.timeLabel.SetText(fmt.Sprintf("00:00 / %s", formatTime(p.duration)))

	// Enable controls
	p.playButton.Enable()
	p.prevButton.Enable()
	p.nextButton.Enable()
}

func (p *YMPlayerGUI) play() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.player == nil || p.playing {
		return
	}

	// Create audio output
	var err error
	p.audioOutput, err = audio.NewStreamingOtoOutput()
	if err != nil {
		dialog.ShowError(err, p.window)
		return
	}

	// Open audio
	if err := p.audioOutput.Open(p.sampleRate, 1, p.bufferSize); err != nil {
		dialog.ShowError(err, p.window)
		p.audioOutput = nil
		return
	}

	p.player.Play()
	p.playing = true
	p.paused = false

	// Update buttons
	p.playButton.Disable()
	p.pauseButton.Enable()
	p.stopButton.Enable()

	// Start playback goroutine
	go p.playbackLoop()
}

func (p *YMPlayerGUI) pause() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.player == nil || !p.playing {
		return
	}

	if p.paused {
		p.player.Play()
		p.paused = false
		p.pauseButton.SetIcon(theme.MediaPauseIcon())
	} else {
		p.player.Pause()
		p.paused = true
		p.pauseButton.SetIcon(theme.MediaPlayIcon())
	}
}

func (p *YMPlayerGUI) stop() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.player == nil {
		return
	}

	// Set flags first
	wasPlaying := p.playing
	p.playing = false
	p.paused = false

	// Stop the player
	p.player.Stop()

	// Close audio output if it was playing
	if wasPlaying && p.audioOutput != nil {
		// Give some time for audio to finish
		time.Sleep(50 * time.Millisecond)
		p.audioOutput.Close()
		p.audioOutput = nil
	}

	// Reset position
	p.position = 0
	p.progressBar.SetValue(0)
	p.timeLabel.SetText(fmt.Sprintf("00:00 / %s", formatTime(p.duration)))

	// Update buttons
	p.playButton.Enable()
	p.pauseButton.Disable()
	p.pauseButton.SetIcon(theme.MediaPauseIcon())
	p.stopButton.Disable()
}

func (p *YMPlayerGUI) playbackLoop() {
	for {
		p.mutex.Lock()
		if !p.playing {
			p.mutex.Unlock()
			break
		}

		// Generate audio
		if !p.player.Compute(p.buffer, len(p.buffer)) {
			if p.repeatMode == RepeatOne {
				// Repeat current track
				p.player.Restart()
			} else if p.repeatMode == RepeatAll || (p.repeatMode == RepeatNone && p.currentIndex < p.playlist.Size()-1) {
				// Play next
				p.mutex.Unlock()
				p.playNext()
				return
			} else {
				// Stop at end
				p.playing = false
				p.mutex.Unlock()
				p.stop()
				break
			}
		}

		// Apply volume
		for i := range p.buffer {
			p.buffer[i] = int16(float64(p.buffer[i]) * p.volume)
		}

		p.mutex.Unlock()

		// Write audio
		if p.audioOutput != nil {
			p.audioOutput.Write(p.buffer)
		}
	}
}

func (p *YMPlayerGUI) playFromIndex(index int) {
	if index < 0 || index >= p.playlist.Size() {
		return
	}

	// Stop current playback
	p.stop()

	// Load new file
	item, _ := p.playlist.Get(index)
	if item != nil {
		data, err := os.ReadFile(item.Path)
		if err != nil {
			dialog.ShowError(err, p.window)
			return
		}

		p.currentIndex = index
		p.loadYMData(item.Path, data)
		p.play()
		p.playlistWidget.Refresh()
	}
}

func (p *YMPlayerGUI) playNext() {
	if p.playlist.Size() == 0 {
		return
	}

	nextIndex := p.currentIndex

	if p.shuffle {
		// Random next
		nextIndex = int(time.Now().UnixNano()) % p.playlist.Size()
	} else {
		// Sequential next
		nextIndex = (p.currentIndex + 1) % p.playlist.Size()

		// Check repeat mode
		if nextIndex == 0 && p.repeatMode == RepeatNone {
			p.stop()
			return
		}
	}

	p.playFromIndex(nextIndex)
}

func (p *YMPlayerGUI) playPrevious() {
	if p.playlist.Size() == 0 {
		return
	}

	prevIndex := p.currentIndex - 1
	if prevIndex < 0 {
		prevIndex = p.playlist.Size() - 1
	}

	p.playFromIndex(prevIndex)
}

func (p *YMPlayerGUI) removeSelected() {
	// Implementation would require tracking selection
}

func (p *YMPlayerGUI) clearPlaylist() {
	dialog.ShowConfirm("Clear Playlist",
		"Are you sure you want to clear the entire playlist?",
		func(ok bool) {
			if ok {
				p.stop()
				p.playlist.Clear()
				p.currentIndex = -1
				p.updatePlaylistLabel()
				p.playlistWidget.Refresh()
				p.playButton.Disable()
			}
		}, p.window)
}

func (p *YMPlayerGUI) moveSelectedUp() {
	// Implementation depends on selection tracking
}

func (p *YMPlayerGUI) moveSelectedDown() {
	// Implementation depends on selection tracking
}

func (p *YMPlayerGUI) savePlaylist() {
	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil || writer == nil {
			return
		}
		writer.Close()

		// Determine format by extension
		path := writer.URI().Path()
		var saveErr error

		if strings.HasSuffix(path, ".m3u") {
			saveErr = p.playlist.SaveM3U(path)
		} else {
			// Default to JSON
			if !strings.HasSuffix(path, ".json") {
				path += ".json"
			}
			saveErr = p.playlist.Save(path)
		}

		if saveErr != nil {
			dialog.ShowError(saveErr, p.window)
		} else {
			dialog.ShowInformation("Success", "Playlist saved successfully", p.window)
		}
	}, p.window)
}

func (p *YMPlayerGUI) loadPlaylist() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		reader.Close()

		path := reader.URI().Path()
		var loadErr error
		var newPlaylist *Playlist

		if strings.HasSuffix(path, ".m3u") {
			newPlaylist, loadErr = LoadM3U(path)
		} else {
			newPlaylist, loadErr = LoadPlaylist(path)
		}

		if loadErr != nil {
			dialog.ShowError(loadErr, p.window)
			return
		}

		// Stop current playback
		p.stop()

		// Replace playlist
		p.playlist = newPlaylist
		p.currentIndex = -1
		p.updatePlaylistLabel()
		p.playlistWidget.Refresh()

		if p.playlist.Size() > 0 {
			p.playButton.Enable()
			p.currentIndex = 0
		}
	}, p.window)
}

func (p *YMPlayerGUI) sortPlaylist(by SortBy) {
	p.playlist.Sort(by)
	p.playlistWidget.Refresh()
}

func (p *YMPlayerGUI) shufflePlaylist() {
	p.playlist.Shuffle()
	p.playlistWidget.Refresh()
}

func (p *YMPlayerGUI) toggleRepeatMode() {
	p.repeatMode = (p.repeatMode + 1) % 3

	switch p.repeatMode {
	case RepeatNone:
		p.repeatButton.SetText("Repeat: Off")
	case RepeatOne:
		p.repeatButton.SetText("Repeat: One")
	case RepeatAll:
		p.repeatButton.SetText("Repeat: All")
	}
}

func (p *YMPlayerGUI) updatePlaylistLabel() {
	total := p.playlist.TotalDuration()
	totalStr := formatTime(total)
	p.playlistLabel.SetText(fmt.Sprintf("Playlist (%d items, %s)",
		p.playlist.Size(), totalStr))
}

func (p *YMPlayerGUI) exportWAV() {
	if p.player == nil {
		dialog.ShowInformation("No file loaded", "Please load a YM file first", p.window)
		return
	}

	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil || writer == nil {
			return
		}
		defer writer.Close()

		// Create progress dialog
		progress := dialog.NewProgress("Exporting to WAV", "Processing...", p.window)
		progress.Show()

		go func() {
			// Export in background
			err := p.exportToWAV(writer.URI().Path(), progress)
			progress.Hide()

			if err != nil {
				dialog.ShowError(err, p.window)
			} else {
				dialog.ShowInformation("Export Complete", "WAV file exported successfully", p.window)
			}
		}()

	}, p.window)
}

func (p *YMPlayerGUI) exportToWAV(filename string, progress dialog.Dialog) error {
	// Stop playback during export
	wasPlaying := p.playing
	if wasPlaying {
		p.stop()
	}

	// Create temporary player for export
	exportPlayer := stsound.CreateWithRate(p.sampleRate)
	defer exportPlayer.Destroy()

	// Reload the file
	data, err := os.ReadFile(p.currentFile)
	if err != nil {
		return err
	}

	if err := exportPlayer.LoadMemory(data); err != nil {
		return err
	}

	// Create WAV output
	wavOut := &WAVOutput{filename: filename}
	if err := wavOut.Open(p.sampleRate, 1, p.bufferSize); err != nil {
		return err
	}
	defer wavOut.Close()

	// Export
	buffer := make([]int16, p.bufferSize)
	exportPlayer.Play()

	info := exportPlayer.GetInfo()
	totalSamples := int(info.MusicTimeInMs) * p.sampleRate / 1000
	processed := 0

	for exportPlayer.Compute(buffer, len(buffer)) {
		wavOut.Write(buffer)
		processed += len(buffer)

		// Update progress
		if totalSamples > 0 {
			prog := float64(processed) / float64(totalSamples)
			if prog > 1.0 {
				prog = 1.0
			}
			// Note: Fyne's progress dialog doesn't expose SetValue
			// This is a limitation of current Fyne version
		}
	}

	return nil
}

func (p *YMPlayerGUI) showAbout() {
	aboutContent := container.NewVBox(
		widget.NewLabelWithStyle("YM Player", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel(""),
		widget.NewLabel("A modern YM music player for Atari ST files"),
		widget.NewLabel(""),
		widget.NewLabel("Based on ST-Sound by Arnaud Carré (Leonard/Oxygene)"),
		widget.NewLabel("Go port with Fyne GUI"),
		widget.NewLabel(""),
		widget.NewLabel("Supports: YM2!, YM3!, YM3b, YM5!, YM6!"),
		widget.NewLabel("With LZH compression support"),
		widget.NewLabel(""),
		container.NewHBox(
			widget.NewLabel("Features:"),
			widget.NewLabel("• Playlist management • Shuffle and repeat modes"),
		),
		container.NewHBox(
			widget.NewLabel(""),
			widget.NewLabel("• WAV export • Cross-platform support"),
		),
	)

	dialog.ShowCustom("About YM Player", "OK", aboutContent, p.window)
}

func (p *YMPlayerGUI) cleanup() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Stop ticker
	if p.ticker != nil {
		p.ticker.Stop()
		close(p.done)
	}

	// Stop playback
	if p.playing {
		p.playing = false
		if p.audioOutput != nil {
			p.audioOutput.Close()
		}
	}

	// Destroy player
	if p.player != nil {
		p.player.Destroy()
	}
}

func (p *YMPlayerGUI) Run() {
	p.window.ShowAndRun()
}

// Helper functions
func formatTime(ms uint32) string {
	seconds := ms / 1000
	minutes := seconds / 60
	seconds %= 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}
