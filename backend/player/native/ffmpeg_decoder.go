package native

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"sync"
	"time"

	"github.com/asticode/go-astiav"
)

// FFmpegDecoder is a universal audio decoder using FFmpeg
// Handles: MP3, AAC, FLAC, OGG, WAV, M4A, ALAC, WMA, and more
type FFmpegDecoder struct {
	reader         io.ReadCloser
	formatContext  *astiav.FormatContext
	codecContext   *astiav.CodecContext
	audioStreamIdx int
	packet         *astiav.Packet
	frame          *astiav.Frame
	sampleRate     int
	numChannels    int
	sampleFormat   astiav.SampleFormat
	mu             sync.Mutex
	eof            bool

	// Buffered decoded samples
	buffer    []byte
	bufferPos int
}

func NewFFmpegDecoder(r io.ReadCloser, url string) (*FFmpegDecoder, error) {
	log.Printf("Creating FFmpeg universal decoder...")

	d := &FFmpegDecoder{
		reader: r,
		buffer: make([]byte, 0),
	}

	// Close the reader - FFmpeg will open the URL itself
	// FFmpeg has its own HTTP implementation and can't use our io.ReadCloser
	if r != nil {
		r.Close()
		d.reader = nil
	}

	// Allocate format context
	d.formatContext = astiav.AllocFormatContext()
	if d.formatContext == nil {
		return nil, fmt.Errorf("failed to allocate format context")
	}

	// Open input - FFmpeg will handle HTTP internally
	if err := d.formatContext.OpenInput(url, nil, nil); err != nil {
		d.formatContext.Free()
		return nil, fmt.Errorf("failed to open input '%s': %w", url, err)
	}

	// Find stream info
	if err := d.formatContext.FindStreamInfo(nil); err != nil {
		d.cleanup()
		return nil, fmt.Errorf("failed to find stream info: %w", err)
	}

	// Find the first audio stream
	d.audioStreamIdx = -1
	for _, stream := range d.formatContext.Streams() {
		if stream.CodecParameters().MediaType() == astiav.MediaTypeAudio {
			d.audioStreamIdx = stream.Index()

			// Get audio parameters
			params := stream.CodecParameters()
			d.sampleRate = params.SampleRate()
			d.numChannels = params.ChannelLayout().Channels()

			// Find decoder
			codec := astiav.FindDecoder(params.CodecID())
			if codec == nil {
				d.cleanup()
				return nil, fmt.Errorf("codec not found for codec ID: %v", params.CodecID())
			}

			// Allocate codec context
			d.codecContext = astiav.AllocCodecContext(codec)
			if d.codecContext == nil {
				d.cleanup()
				return nil, fmt.Errorf("failed to allocate codec context")
			}

			// Copy codec parameters to context
			if err := params.ToCodecContext(d.codecContext); err != nil {
				d.cleanup()
				return nil, fmt.Errorf("failed to copy codec parameters: %w", err)
			}

			// Open codec
			if err := d.codecContext.Open(codec, nil); err != nil {
				d.cleanup()
				return nil, fmt.Errorf("failed to open codec: %w", err)
			}

			d.sampleFormat = d.codecContext.SampleFormat()

			log.Printf("FFmpeg decoder ready: %d Hz, %d channels, codec: %s, sample format: %s",
				d.sampleRate, d.numChannels, codec.Name(), d.sampleFormat.Name())
			break
		}
	}

	if d.audioStreamIdx < 0 {
		d.cleanup()
		return nil, fmt.Errorf("no audio stream found")
	}

	// Allocate packet and frame
	d.packet = astiav.AllocPacket()
	if d.packet == nil {
		d.cleanup()
		return nil, fmt.Errorf("failed to allocate packet")
	}

	d.frame = astiav.AllocFrame()
	if d.frame == nil {
		d.cleanup()
		return nil, fmt.Errorf("failed to allocate frame")
	}

	return d, nil
}

func (d *FFmpegDecoder) Read(p []byte) (n int, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Sanity checks
	if d.formatContext == nil || d.codecContext == nil || d.packet == nil || d.frame == nil {
		return 0, fmt.Errorf("decoder not properly initialized")
	}

	bytesRead := 0

	for bytesRead < len(p) {
		// First, drain any buffered samples
		if d.bufferPos < len(d.buffer) {
			copied := copy(p[bytesRead:], d.buffer[d.bufferPos:])
			bytesRead += copied
			d.bufferPos += copied

			if bytesRead >= len(p) {
				return bytesRead, nil
			}
		}

		// Need more data - decode next frame
		if d.eof {
			if bytesRead == 0 {
				return 0, io.EOF
			}
			return bytesRead, nil
		}

		// Read packets until we get a frame
		gotFrame := false
		for !gotFrame {
			// Read packet
			if err := d.formatContext.ReadFrame(d.packet); err != nil {
				if err == astiav.ErrEof {
					d.eof = true
					// Flush decoder
					if sendErr := d.codecContext.SendPacket(nil); sendErr != nil {
						log.Printf("Error flushing decoder: %v", sendErr)
					}
					break
				} else {
					return bytesRead, fmt.Errorf("failed to read frame: %w", err)
				}
			}

			// Only process audio packets from our stream
			if d.packet.StreamIndex() != d.audioStreamIdx {
				d.packet.Unref()
				if d.eof {
					break
				}
				continue
			}

			// Send packet to decoder
			if err := d.codecContext.SendPacket(d.packet); err != nil {
				d.packet.Unref()
				return bytesRead, fmt.Errorf("failed to send packet: %w", err)
			}
			d.packet.Unref()

			// Receive decoded frame
			if err := d.codecContext.ReceiveFrame(d.frame); err != nil {
				if err == astiav.ErrEagain {
					// Need more packets
					continue
				} else if err == astiav.ErrEof {
					d.eof = true
					break
				}
				return bytesRead, fmt.Errorf("failed to receive frame: %w", err)
			}

			gotFrame = true
		}

		if !gotFrame {
			if bytesRead == 0 {
				return 0, io.EOF
			}
			return bytesRead, nil
		}

		// Convert frame to interleaved PCM samples (16-bit signed)
		samples := d.convertFrameToPCM()
		d.buffer = samples
		d.bufferPos = 0
	}

	return bytesRead, nil
}

func (d *FFmpegDecoder) convertFrameToPCM() []byte {
	nbSamples := d.frame.NbSamples()

	// Check if we need to convert from float to int16
	sampleFormat := d.frame.SampleFormat()

	// For float formats (FLT, FLTP), we need manual conversion
	if sampleFormat == astiav.SampleFormatFlt || sampleFormat == astiav.SampleFormatFltp {
		return d.convertFloatFrameToInt16()
	}

	// For int16 formats (S16, S16P), use direct buffer copy
	// Calculate buffer size for int16 output
	bytesPerSample := 2 // 16-bit = 2 bytes
	bufSize := nbSamples * d.numChannels * bytesPerSample

	// Allocate output buffer
	output := make([]byte, bufSize)

	// Copy samples to buffer (handles interleaving automatically)
	n, err := d.frame.SamplesCopyToBuffer(output, 1)
	if err != nil {
		log.Printf("Error copying samples to buffer: %v", err)
		return nil
	}

	return output[:n]
}

func (d *FFmpegDecoder) convertFloatFrameToInt16() []byte {
	nbSamples := d.frame.NbSamples()
	bytesPerSample := 2 // Output is 16-bit
	outputSize := nbSamples * d.numChannels * bytesPerSample
	output := make([]byte, outputSize)

	sampleFormat := d.frame.SampleFormat()

	if sampleFormat == astiav.SampleFormatFlt {
		// Interleaved float32
		// Get raw float32 data
		floatSize := nbSamples * d.numChannels * 4 // float32 = 4 bytes
		floatBuf := make([]byte, floatSize)

		_, err := d.frame.SamplesCopyToBuffer(floatBuf, 1)
		if err != nil {
			log.Printf("Error copying float samples: %v", err)
			return nil
		}

		// Convert float32 to int16
		outPos := 0
		for i := 0; i < len(floatBuf); i += 4 {
			// Read float32 (little-endian)
			floatBits := binary.LittleEndian.Uint32(floatBuf[i : i+4])
			floatVal := math.Float32frombits(floatBits)

			// Clamp to [-1.0, 1.0] and convert to int16
			if floatVal > 1.0 {
				floatVal = 1.0
			} else if floatVal < -1.0 {
				floatVal = -1.0
			}

			intSample := int16(floatVal * 32767.0)

			// Write int16 (little-endian)
			binary.LittleEndian.PutUint16(output[outPos:outPos+2], uint16(intSample))
			outPos += 2
		}
	} else if sampleFormat == astiav.SampleFormatFltp {
		// Planar float32 - channels are stored separately, need to interleave
		floatBytesPerChannel := nbSamples * 4 // float32 = 4 bytes

		// The SamplesCopyToBuffer returns data in planar layout:
		// [all samples for channel 0][all samples for channel 1]...
		totalFloatSize := floatBytesPerChannel * d.numChannels
		tempBuf := make([]byte, totalFloatSize)

		_, err := d.frame.SamplesCopyToBuffer(tempBuf, 1)
		if err != nil {
			log.Printf("Error copying planar float samples: %v", err)
			return nil
		}

		// Interleave and convert to int16
		outPos := 0
		for sampleIdx := 0; sampleIdx < nbSamples; sampleIdx++ {
			for ch := 0; ch < d.numChannels; ch++ {
				// Calculate position in planar buffer:
				// Channel data starts at: ch * floatBytesPerChannel
				// Sample position within channel: sampleIdx * 4
				byteIdx := (ch * floatBytesPerChannel) + (sampleIdx * 4)

				// Read float32
				floatBits := binary.LittleEndian.Uint32(tempBuf[byteIdx : byteIdx+4])
				floatVal := math.Float32frombits(floatBits)

				// Clamp and convert to int16
				if floatVal > 1.0 {
					floatVal = 1.0
				} else if floatVal < -1.0 {
					floatVal = -1.0
				}

				intSample := int16(floatVal * 32767.0)

				// Write int16
				binary.LittleEndian.PutUint16(output[outPos:outPos+2], uint16(intSample))
				outPos += 2
			}
		}
	}

	return output
}

// floatFromBits converts a uint32 bit pattern to float32
func floatFromBits(bits uint32) float32 {
	return math.Float32frombits(bits)
}

func (d *FFmpegDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cleanup()
	if d.reader != nil {
		return d.reader.Close()
	}
	return nil
}

func (d *FFmpegDecoder) cleanup() {
	if d.frame != nil {
		d.frame.Free()
		d.frame = nil
	}
	if d.packet != nil {
		d.packet.Free()
		d.packet = nil
	}
	if d.codecContext != nil {
		d.codecContext.Free()
		d.codecContext = nil
	}
	if d.formatContext != nil {
		d.formatContext.CloseInput()
		d.formatContext.Free()
		d.formatContext = nil
	}
}

func (d *FFmpegDecoder) SampleRate() int {
	return d.sampleRate
}

func (d *FFmpegDecoder) NumChannels() int {
	return d.numChannels
}

func (d *FFmpegDecoder) Seek(offset time.Duration) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Convert time to timestamp
	stream := d.formatContext.Streams()[d.audioStreamIdx]
	timestamp := int64(offset.Seconds() * float64(stream.TimeBase().Den()) / float64(stream.TimeBase().Num()))

	// Seek to timestamp
	if err := d.formatContext.SeekFrame(d.audioStreamIdx, timestamp, astiav.NewSeekFlags(astiav.SeekFlagBackward)); err != nil {
		return fmt.Errorf("seek failed: %w", err)
	}

	// Flush codec buffers by sending nil packet and receiving remaining frames
	d.codecContext.SendPacket(nil)
	// Drain the decoder
	for {
		err := d.codecContext.ReceiveFrame(d.frame)
		if err != nil {
			break
		}
	}

	// Clear internal buffer
	d.buffer = d.buffer[:0]
	d.bufferPos = 0
	d.eof = false

	return nil
}

func (d *FFmpegDecoder) Length() time.Duration {
	if d.formatContext == nil {
		return 0
	}

	// Get duration from format context (in AV_TIME_BASE units)
	duration := d.formatContext.Duration()
	if duration > 0 {
		return time.Duration(duration) * time.Microsecond
	}

	return 0
}
