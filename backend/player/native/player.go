package native

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dweymouth/supersonic/backend/mediaprovider"
	"github.com/dweymouth/supersonic/backend/player"
	"github.com/ebitengine/oto/v3"
)

// Error returned by many Player functions if called before the player has been initialized.
var ErrUninitialized = errors.New("native player uninitialized")
var ErrNoDecoder = errors.New("no suitable decoder found for file")

// Player is a pure Go audio player implementation
type Player struct {
	player.BasePlayerCallbackImpl

	ctx            context.Context
	cancel         context.CancelFunc
	initialized    bool
	vol            int
	status         player.Status
	seeking        bool
	curPlaylistPos int
	lenPlaylist    int
	prePausedState player.State

	// Audio playback
	otoContext *oto.Context
	otoPlayer  *oto.Player
	decoder    Decoder

	// Track management
	currentURL      string
	nextURL         string
	currentMetadata mediaprovider.MediaItemMetadata
	nextMetadata    mediaprovider.MediaItemMetadata

	// Synchronization
	mu              sync.RWMutex
	playbackMu      sync.Mutex
	stopRequested   bool
	pauseRequested  bool
	seekTarget      float64
	seekRequested   bool
	startTime       float64
	trackStartTime  time.Time
	pausedAt        time.Duration
	pausedDuration  time.Duration
	audioBufferSize int
}

// SupportedFormats returns the list of audio formats supported by the native player
var SupportedFormats = []string{"mp3", "flac", "ogg", "vorbis", "wav", "wave"}

// IsFormatSupported checks if the given format is supported by the native player
func IsFormatSupported(format string) bool {
	format = strings.ToLower(strings.TrimPrefix(format, "."))
	for _, supported := range SupportedFormats {
		if supported == format {
			return true
		}
	}
	return false
}

// New returns a new native Go audio player
func New() *Player {
	return &Player{
		vol:             -1, // use 100 in Init
		audioBufferSize: 8192,
	}
}

// Init initializes the player and makes it ready for playback
func (p *Player) Init() error {
	if p.initialized {
		return nil
	}

	if p.vol < 0 {
		p.vol = 100
	}

	// Create base context for the player
	ctx, cancel := context.WithCancel(context.Background())
	p.ctx = ctx
	p.cancel = cancel

	// Initialize oto context (will be set when playback starts with proper sample rate)
	// We'll create this lazily when we know the audio format

	p.initialized = true
	return nil
}

// PlayFile plays the specified file, clearing the previous play queue, if any
func (p *Player) PlayFile(url string, metadata mediaprovider.MediaItemMetadata, startTime float64) error {
	if !p.initialized {
		return ErrUninitialized
	}

	p.mu.Lock()
	p.currentURL = url
	p.currentMetadata = metadata
	p.startTime = startTime
	p.lenPlaylist = 1
	p.curPlaylistPos = 0
	p.mu.Unlock()

	if err := p.startPlayback(url, startTime); err != nil {
		return err
	}

	p.setState(player.Playing)
	p.InvokeOnTrackChange()

	return nil
}

// SetNextFile sets the next file to play
func (p *Player) SetNextFile(url string, metadata mediaprovider.MediaItemMetadata) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.lenPlaylist > p.curPlaylistPos+1 {
		p.lenPlaylist--
	}

	if url == "" {
		p.nextURL = ""
		p.nextMetadata = mediaprovider.MediaItemMetadata{}
		return nil
	}

	p.nextURL = url
	p.nextMetadata = metadata
	p.lenPlaylist++
	return nil
}

// Stop stops playback and clears the play queue
func (p *Player) Stop(_ bool) error {
	if !p.initialized {
		return ErrUninitialized
	}

	p.mu.Lock()
	p.stopRequested = true
	p.mu.Unlock()

	p.stopPlayback()

	p.mu.Lock()
	p.lenPlaylist = 0
	p.curPlaylistPos = 0
	p.stopRequested = false
	p.mu.Unlock()

	p.setState(player.Stopped)
	return nil
}

// Pause pauses playback
func (p *Player) Pause() error {
	if !p.initialized {
		return ErrUninitialized
	}

	p.mu.Lock()
	if p.status.State != player.Playing {
		p.mu.Unlock()
		return nil
	}
	p.pauseRequested = true
	p.mu.Unlock()

	p.playbackMu.Lock()
	if p.otoPlayer != nil {
		p.pausedAt = time.Since(p.trackStartTime) - p.pausedDuration
		p.otoPlayer.Pause()
	}
	p.playbackMu.Unlock()

	p.prePausedState = p.status.State
	p.setState(player.Paused)

	return nil
}

// Continue continues playback
func (p *Player) Continue() error {
	if !p.initialized {
		return ErrUninitialized
	}

	p.mu.Lock()
	if p.status.State != player.Paused {
		p.mu.Unlock()
		return nil
	}
	p.pauseRequested = false
	p.mu.Unlock()

	p.playbackMu.Lock()
	if p.pausedAt > 0 {
		p.pausedDuration += time.Since(p.trackStartTime) - p.pausedAt
		p.pausedAt = 0
	}
	if p.otoPlayer != nil {
		p.otoPlayer.Play()
	}
	p.playbackMu.Unlock()

	p.setState(p.prePausedState)

	return nil
}

// SeekSeconds seeks to the specified position in seconds
func (p *Player) SeekSeconds(secs float64) error {
	if !p.initialized {
		return ErrUninitialized
	}

	p.mu.Lock()
	p.seekTarget = secs
	p.seekRequested = true
	p.seeking = true
	p.mu.Unlock()

	// Seeking will be handled in the playback loop
	go func() {
		time.Sleep(100 * time.Millisecond)
		p.mu.Lock()
		p.seeking = false
		p.mu.Unlock()
		p.InvokeOnSeek()
	}()

	return nil
}

// IsSeeking returns true if a seek is currently in progress
func (p *Player) IsSeeking() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.seeking && p.status.State == player.Playing
}

// SetVolume sets the volume (0-100)
func (p *Player) SetVolume(vol int) error {
	if vol > 100 {
		vol = 100
	} else if vol < 0 {
		vol = 0
	}

	p.mu.Lock()
	p.vol = vol
	p.mu.Unlock()

	// Volume will be applied in the playback loop
	return nil
}

// GetVolume returns the current volume
func (p *Player) GetVolume() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.vol
}

// GetStatus returns the current player status
func (p *Player) GetStatus() player.Status {
	p.mu.RLock()
	defer p.mu.RUnlock()

	status := p.status
	if p.status.State == player.Playing && p.decoder != nil {
		elapsed := time.Since(p.trackStartTime) - p.pausedDuration
		status.TimePos = p.startTime + elapsed.Seconds()
		status.Duration = p.currentMetadata.Duration.Seconds()
	}

	return status
}

// Destroy destroys the player and releases resources
func (p *Player) Destroy() {
	if !p.initialized {
		return
	}

	p.stopPlayback()

	if p.cancel != nil {
		p.cancel()
	}

	p.playbackMu.Lock()
	if p.otoContext != nil {
		if err := p.otoContext.Suspend(); err != nil {
			log.Printf("error suspending oto context: %v", err)
		}
	}
	p.playbackMu.Unlock()

	p.initialized = false
}

// setState sets the state and invokes callbacks
func (p *Player) setState(s player.State) {
	p.mu.Lock()
	oldState := p.status.State
	p.status.State = s
	p.mu.Unlock()

	if s == player.Playing && oldState != player.Playing {
		p.InvokeOnPlaying()
	} else if s == player.Paused && oldState != player.Paused {
		p.InvokeOnPaused()
	} else if s == player.Stopped && oldState != player.Stopped {
		p.InvokeOnStopped()
	}
}

// startPlayback starts playing the given URL
func (p *Player) startPlayback(url string, startTime float64) error {
	p.stopPlayback()

	startTimer := time.Now()
	log.Printf("Starting playback...")

	// Open the audio file/stream
	var reader io.ReadCloser
	var contentType string
	var err error

	if isURL(url) {
		// Use HTTPSeeker for HTTP streams to support seeking via range requests
		seeker, err := NewHTTPSeeker(url)
		if err != nil {
			return fmt.Errorf("failed to open stream: %w", err)
		}
		reader = seeker
		contentType = seeker.ContentType()
		log.Printf("HTTP stream opened in %v, Content-Type: %s", time.Since(startTimer), contentType)
	} else {
		file, err := os.Open(url)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		reader = file
	}

	// Create FFmpeg-based decoder
	decoderStart := time.Now()

	decoder, err := NewDecoder(reader, url, contentType)
	if err != nil {
		reader.Close()
		return fmt.Errorf("failed to create decoder: %w", err)
	}
	log.Printf("Decoder created in %v", time.Since(decoderStart))

	p.playbackMu.Lock()
	p.decoder = decoder
	p.trackStartTime = time.Now()
	p.pausedDuration = 0
	p.pausedAt = 0

	// Initialize oto context if needed
	sampleRate := decoder.SampleRate()
	numChannels := decoder.NumChannels()

	if p.otoContext == nil {
		op := &oto.NewContextOptions{
			SampleRate:   sampleRate,
			ChannelCount: numChannels,
			Format:       oto.FormatSignedInt16LE,
		}

		var ready chan struct{}
		var err error
		p.otoContext, ready, err = oto.NewContext(op)
		if err != nil {
			p.playbackMu.Unlock()
			decoder.Close()
			reader.Close()
			return fmt.Errorf("failed to create oto context: %w", err)
		}
		<-ready
	}

	// Create oto player
	p.otoPlayer = p.otoContext.NewPlayer(decoder)
	p.playbackMu.Unlock()

	// Start playback monitoring
	go p.monitorPlayback()

	// Handle start time seeking if needed
	if startTime > 0 {
		if err := p.seekTo(startTime); err != nil {
			log.Printf("failed to seek to start time: %v", err)
		}
	}

	// Start playing
	p.playbackMu.Lock()
	if p.otoPlayer != nil {
		p.otoPlayer.Play()
	}
	p.playbackMu.Unlock()

	return nil
}

// stopPlayback stops the current playback
func (p *Player) stopPlayback() {
	p.playbackMu.Lock()
	defer p.playbackMu.Unlock()

	if p.otoPlayer != nil {
		p.otoPlayer.Close()
		p.otoPlayer = nil
	}

	if p.decoder != nil {
		p.decoder.Close()
		p.decoder = nil
	}
}

// seekTo seeks to a specific position
func (p *Player) seekTo(secs float64) error {
	p.playbackMu.Lock()
	defer p.playbackMu.Unlock()

	if p.decoder == nil {
		return errors.New("no decoder available")
	}

	if err := p.decoder.Seek(time.Duration(secs * float64(time.Second))); err != nil {
		return fmt.Errorf("seek failed: %w", err)
	}

	p.trackStartTime = time.Now()
	p.pausedDuration = 0
	p.startTime = secs

	return nil
}

// monitorPlayback monitors playback and handles track completion
func (p *Player) monitorPlayback() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.mu.RLock()
			stopReq := p.stopRequested
			pauseReq := p.pauseRequested
			seekReq := p.seekRequested
			seekTarget := p.seekTarget
			state := p.status.State
			p.mu.RUnlock()

			if stopReq {
				return
			}

			// Handle pause
			if pauseReq && state == player.Playing {
				continue
			}

			// Handle seek
			if seekReq {
				p.mu.Lock()
				p.seekRequested = false
				p.mu.Unlock()

				if err := p.seekTo(seekTarget); err != nil {
					log.Printf("seek error: %v", err)
				}
			}

			// Check if track has finished
			p.playbackMu.Lock()
			isPlaying := p.otoPlayer != nil && p.otoPlayer.IsPlaying()
			p.playbackMu.Unlock()

			if !isPlaying && state == player.Playing && !pauseReq {
				// Track finished, play next if available
				p.mu.Lock()
				nextURL := p.nextURL
				nextMeta := p.nextMetadata
				p.curPlaylistPos++
				p.mu.Unlock()

				if nextURL != "" {
					p.mu.Lock()
					p.currentURL = nextURL
					p.currentMetadata = nextMeta
					p.nextURL = ""
					p.nextMetadata = mediaprovider.MediaItemMetadata{}
					p.mu.Unlock()

					if err := p.startPlayback(nextURL, 0); err != nil {
						log.Printf("failed to play next track: %v", err)
						p.Stop(false)
						return
					}

					p.InvokeOnTrackChange()
				} else {
					p.Stop(false)
					return
				}
			}
		}
	}
}

// isURL checks if a string is a URL
func isURL(s string) bool {
	return len(s) > 7 && (s[:7] == "http://" || s[:8] == "https://")
}
