package native

import (
	"fmt"
	"io"
	"log"
	"time"
)

// Decoder is an interface for audio decoders
type Decoder interface {
	io.Reader
	io.Closer
	SampleRate() int
	NumChannels() int
	Seek(time.Duration) error
	Length() time.Duration
}

// NewDecoder creates a new decoder based on the file extension, URL parameters, or Content-Type
// Now uses FFmpeg for universal format support (MP3, AAC, M4A, FLAC, OGG, WAV, etc.)
func NewDecoder(r io.ReadCloser, filename string, contentType string) (Decoder, error) {
	log.Printf("Creating decoder for: %s (Content-Type: %s)", filename, contentType)

	// FFmpeg can handle URLs directly, so we pass the filename/URL
	// The reader 'r' will be closed by FFmpeg when it opens the URL itself
	decoder, err := NewFFmpegDecoder(r, filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create FFmpeg decoder: %w", err)
	}

	return decoder, nil
}
