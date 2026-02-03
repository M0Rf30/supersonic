package native

import (
	"fmt"
	"io"
	"net/http"
)

// HTTPSeeker wraps an HTTP URL to provide ReadSeeker interface using HTTP range requests
type HTTPSeeker struct {
	url           string
	currentPos    int64
	contentLength int64
	contentType   string
	reader        io.ReadCloser
	client        *http.Client
}

// NewHTTPSeeker creates a new HTTPSeeker for the given URL
func NewHTTPSeeker(url string) (*HTTPSeeker, error) {
	client := &http.Client{}

	// Do a HEAD request to get content length and type (fast)
	resp, err := client.Head(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get content length: %w", err)
	}
	defer resp.Body.Close()

	contentLength := resp.ContentLength
	// Content length might be -1 for chunked encoding, that's ok

	contentType := resp.Header.Get("Content-Type")

	hs := &HTTPSeeker{
		url:           url,
		contentLength: contentLength,
		contentType:   contentType,
		currentPos:    0,
		client:        client,
	}

	// Open initial connection (fast - doesn't wait for full download)
	if err := hs.openReader(0); err != nil {
		return nil, err
	}

	return hs, nil
}

// ContentType returns the Content-Type header from the HTTP response
func (hs *HTTPSeeker) ContentType() string {
	return hs.contentType
}

// openReader opens a reader at the specified position
func (hs *HTTPSeeker) openReader(pos int64) error {
	// Close existing reader if any
	if hs.reader != nil {
		hs.reader.Close()
	}

	req, err := http.NewRequest("GET", hs.url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set range header
	if pos > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", pos))
	}

	resp, err := hs.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}

	// Check for valid response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	hs.reader = resp.Body
	hs.currentPos = pos

	return nil
}

// Read implements io.Reader
func (hs *HTTPSeeker) Read(p []byte) (n int, err error) {
	if hs.reader == nil {
		return 0, fmt.Errorf("no active reader")
	}

	n, err = hs.reader.Read(p)
	hs.currentPos += int64(n)
	return n, err
}

// Seek implements io.Seeker
func (hs *HTTPSeeker) Seek(offset int64, whence int) (int64, error) {
	var newPos int64

	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = hs.currentPos + offset
	case io.SeekEnd:
		newPos = hs.contentLength + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}

	if newPos < 0 {
		return 0, fmt.Errorf("negative position")
	}
	if newPos > hs.contentLength {
		newPos = hs.contentLength
	}

	// Only reopen if we're seeking to a different position
	if newPos != hs.currentPos {
		if err := hs.openReader(newPos); err != nil {
			return hs.currentPos, err
		}
	}

	return hs.currentPos, nil
}

// Close implements io.Closer
func (hs *HTTPSeeker) Close() error {
	if hs.reader != nil {
		return hs.reader.Close()
	}
	return nil
}
