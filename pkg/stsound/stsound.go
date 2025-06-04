package stsound

// StSound - Main API interface matching the C API
type StSound struct {
	music *CYmMusic
}

// Create creates a new StSound instance
func Create() *StSound {
	return &StSound{
		music: NewYmMusic(44100),
	}
}

// CreateWithRate creates a new StSound instance with specific replay rate
func CreateWithRate(replayRate int) *StSound {
	return &StSound{
		music: NewYmMusic(replayRate),
	}
}

// Destroy releases the StSound instance
func (s *StSound) Destroy() {
	if s.music != nil {
		s.music.UnLoad()
		s.music = nil
	}
}

// Load loads a YM file from disk
func (s *StSound) Load(fileName string) error {
	return s.music.Load(fileName)
}

// LoadMemory loads a YM file from memory
func (s *StSound) LoadMemory(data []byte) error {
	return s.music.LoadMemory(data)
}

// Compute renders audio samples
func (s *StSound) Compute(buffer []int16, nbSamples int) bool {
	// Créer un buffer temporaire pour les échantillons YM
	ymBuffer := make([]YmSample, nbSamples)
	result := s.music.Update(ymBuffer, nbSamples) == YmTrue

	// Copier le résultat dans le buffer original sans amplification supplémentaire
	for i := 0; i < nbSamples; i++ {
		buffer[i] = int16(ymBuffer[i])
	}

	return result
}

// SetLoopMode enables/disables loop mode
func (s *StSound) SetLoopMode(loop bool) {
	s.music.SetLoopMode(YmBool(loop))
}

// GetLastError returns the last error message
func (s *StSound) GetLastError() string {
	return s.music.GetLastError()
}

// GetRegister reads a YM register value
func (s *StSound) GetRegister(reg int) int {
	return s.music.ReadYmRegister(reg)
}

// GetInfo returns music information
func (s *StSound) GetInfo() *YmMusicInfo {
	return s.music.GetMusicInfo()
}

// Play starts playback
func (s *StSound) Play() {
	s.music.Play()
}

// Pause pauses playback
func (s *StSound) Pause() {
	s.music.Pause()
}

// Stop stops playback
func (s *StSound) Stop() {
	s.music.Stop()
}

// IsOver checks if music has finished
func (s *StSound) IsOver() bool {
	return s.music.GetMusicOver() == YmTrue
}

// IsSeekable checks if the music supports seeking
func (s *StSound) IsSeekable() bool {
	return s.music.IsSeekable() == YmTrue
}

// GetPos returns current position in milliseconds
func (s *StSound) GetPos() uint32 {
	return uint32(s.music.GetPos())
}

// Seek seeks to a specific time in milliseconds
func (s *StSound) Seek(timeInMs uint32) {
	s.music.SetMusicTime(YmU32(timeInMs))
}

// Restart restarts the music from the beginning
func (s *StSound) Restart() {
	s.music.Restart()
}

// SetLowpassFilter enables/disables the lowpass filter
func (s *StSound) SetLowpassFilter(active bool) {
	s.music.SetLowpassFilter(YmBool(active))
}
