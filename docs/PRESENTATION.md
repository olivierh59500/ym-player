# YM Player presentation

The presentation shows the same YM replay engine through the command-line
player and the Fyne desktop application. The screenshots show terminal metadata
and playback, the graphical player, and a playlist containing several tracks.

[![Animated terminal and Fyne player preview](media/preview.gif)](https://github.com/olivierh59500/ym-player/raw/refs/heads/main/docs/media/presentation.mp4)

**[Watch or download the 54-second presentation with sound — MP4](https://github.com/olivierh59500/ym-player/raw/refs/heads/main/docs/media/presentation.mp4)**

The animated image is silent. Play the MP4 to hear the YM music. The presentation
starts with 18 seconds of command-line output, then shows 36 seconds of the Fyne
application. English captions are included directly in the video. An [English subtitle file](media/presentation.en.srt) is supplied
for players that accept external subtitles.

The terminal session inspects and loops **Goldrunner**, credited in the file to
Rob Hubbard, at 48 kHz. The graphical sequence uses Goldrunner and **Trapped in
China**, credited to Jochen Hippel (Mad Max). It shows track selection, metadata,
pause/resume, volume adjustment to 45%, low-pass filtering, the next-track
button, track looping, and playlist repeat options.

## Try the command-line workflow

From the repository root:

```sh
# Build the command-line player.
go build -o ymplayer ./cmd/ymplayer

# Read a track's format and metadata without opening an audio device.
./ymplayer -info /path/to/track.ym

# Play through the desktop audio output.
./ymplayer /path/to/track.ym

# Export mono 16-bit PCM audio to a WAV file.
./ymplayer -output wav -wav /path/to/track.wav /path/to/track.ym
```

Replace the example paths with your own files. Flags go before the input path.
The default output rate is 44,100 Hz with a 2,048-sample buffer. Use `-rate 48000`
for a 48 kHz export. `-lowpass=false` disables filtering; `-volume` and `-gain`
scale the rendered samples. `Ctrl+C` stops playback. A WAV export without
`-wav` uses the input basename with a `.wav` extension.

[![Terminal metadata and playback progress](media/cli.png)](media/cli.png)

## Try the graphical workflow

```sh
# Build the Fyne application.
go build -tags gui -o ymplayer-gui ./cmd/ymplayer-gui

# Load a track into the graphical player.
./ymplayer-gui /path/to/track.ym
```

Press **Play** after opening a file from the command line. **File → Add Files**
and **Add Folder** append tracks to the playlist. Selecting a row loads and
starts it. **Previous** and **Next** navigate between tracks, while the volume
slider, **Loop Track**, **Low-pass Filter**, **Shuffle**, and **Repeat** controls
adjust playback.

[![Fyne playback controls and metadata](media/gui-playback.png)](media/gui-playback.png)

The **Pause** button suspends playback without unloading the track; pressing it
again resumes.

[![Paused Fyne playback](media/gui-paused.png)](media/gui-paused.png)

Use the **Playlist** menu to sort by title, author, or duration, or shuffle the
order. The **File** menu saves and loads JSON/M3U playlists and exports the
loaded track to WAV. **Clear** empties the playlist after confirmation.

[![Fyne playlist and playback options](media/gui-playlist.png)](media/gui-playlist.png)

JSON retains the stored metadata and paths. The M3U reader accepts `.ym` paths
as written; relative paths are interpreted from the application working
directory. The disabled **Remove**, **Move Up**, and **Move Down** buttons are
placeholders in the current interface. Previous/Next may leave the selected-row
highlight on an earlier entry; consult the Now Playing panel for the loaded
track. The player displays metadata and time progress rather than cover art or
a waveform.

## Media files

| File | Contents |
| --- | --- |
| `media/cli.png` | Command-line metadata and playback capture |
| `media/gui-playback.png` | Fyne player with a loaded track |
| `media/gui-paused.png` | Paused playback and retained track metadata |
| `media/gui-playlist.png` | Fyne playlist view |
| `media/preview.gif` | Silent animated preview |
| `media/presentation.mp4` | 54-second presentation with music |
| `media/presentation.en.srt` | English subtitles |

The final MP4 uses H.264 video at 1280×900 and AAC audio at 48 kHz. The underlying
YM replay engine renders mono signed 16-bit PCM at 44.1 kHz by default; the media
encoding settings are separate from those player defaults.

The terminal images render recorded pseudo-terminal output from the real CLI.
The graphical images record the Fyne application's own canvas. Audio comes
from the YM replay engine.

See the [main README](../README.md) for supported YM/MIX/YMT formats,
compression, chip emulation, platform requirements, and the compatibility audit.
