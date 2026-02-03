package native

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"sync"
)

// StreamSeeker provides a ReadSeeker interface with progressive buffering
// This allows decoders to work while the stream is still downloading
type StreamSeeker struct {
	reader io.ReadCloser
	buffer *bytes.Buffer
	mu     sync.RWMutex
	pos    int64
	done   bool
	err    error
}

// NewStreamSeeker creates a new StreamSeeker that buffers in the background
func NewStreamSeeker(r io.ReadCloser) *StreamSeeker {
	ss := &StreamSeeker{
		reader: r,
		buffer: new(bytes.Buffer),
		pos:    0,
		done:   false,
	}

	// Start background buffering
	go ss.bufferInBackground()

	return ss
}

// bufferInBackground reads from the source and buffers data
func (ss *StreamSeeker) bufferInBackground() {
	defer func() {
		ss.mu.Lock()
		ss.done = true
		log.Printf("StreamSeeker: buffering complete, total bytes: %d", ss.buffer.Len())
		ss.mu.Unlock()
	}()

	buf := make([]byte, 32*1024) // 32KB chunks
	totalRead := 0
	for {
		n, err := ss.reader.Read(buf)
		if n > 0 {
			ss.mu.Lock()
			ss.buffer.Write(buf[:n])
			totalRead += n
			ss.mu.Unlock()

			// Log progress every 1MB
			if totalRead%(1024*1024) == 0 {
				log.Printf("StreamSeeker: buffered %d MB", totalRead/(1024*1024))
			}
		}
		if err != nil {
			if err != io.EOF {
				ss.mu.Lock()
				ss.err = err
				ss.mu.Unlock()
				log.Printf("StreamSeeker background error: %v", err)
			}
			break
		}
	}
}

// Read implements io.Reader
func (ss *StreamSeeker) Read(p []byte) (n int, err error) {
	for {
		ss.mu.RLock()
		available := int64(ss.buffer.Len()) - ss.pos
		isDone := ss.done
		bufErr := ss.err
		ss.mu.RUnlock()

		// If we have data available, read it
		if available > 0 {
			ss.mu.Lock()
			// Get a reader for the buffered data
			bufBytes := ss.buffer.Bytes()
			if ss.pos >= int64(len(bufBytes)) {
				ss.mu.Unlock()
				if isDone {
					return 0, io.EOF
				}
				continue
			}

			toRead := len(p)
			if int64(toRead) > available {
				toRead = int(available)
			}

			copy(p, bufBytes[ss.pos:ss.pos+int64(toRead)])
			ss.pos += int64(toRead)
			ss.mu.Unlock()
			return toRead, nil
		}

		// No data available
		if isDone {
			if bufErr != nil {
				return 0, bufErr
			}
			return 0, io.EOF
		}

		// Wait a bit for more data
		// In a production system, you'd use a condition variable
		// For now, just yield the CPU briefly
		// (no unlock needed here - we already unlocked at the top of the loop)
	}
}

// Seek implements io.Seeker
func (ss *StreamSeeker) Seek(offset int64, whence int) (int64, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	var newPos int64
	bufLen := int64(ss.buffer.Len())

	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = ss.pos + offset
	case io.SeekEnd:
		// We can only seek to end if buffering is complete
		if !ss.done {
			return ss.pos, fmt.Errorf("cannot seek to end while buffering")
		}
		newPos = bufLen + offset
	default:
		return ss.pos, fmt.Errorf("invalid whence: %d", whence)
	}

	if newPos < 0 {
		return ss.pos, fmt.Errorf("negative position")
	}

	// Can only seek within buffered range
	if newPos > bufLen {
		if ss.done {
			newPos = bufLen
		} else {
			return ss.pos, fmt.Errorf("seek beyond buffered data (want %d, have %d)", newPos, bufLen)
		}
	}

	ss.pos = newPos
	return ss.pos, nil
}

// Close implements io.Closer
func (ss *StreamSeeker) Close() error {
	if ss.reader != nil {
		return ss.reader.Close()
	}
	return nil
}
