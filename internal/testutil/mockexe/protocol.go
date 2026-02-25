package mockexe

import (
	"encoding/binary"
	"io"
	"sync"
)

// Frame types for the binary protocol between callback client and server.
const (
	FrameInit   byte = 0x01 // client→server: JSON {args, env}
	FrameStdin  byte = 0x02 // client→server: raw bytes (0-length = EOF)
	FrameStdout byte = 0x03 // server→client: raw bytes (0-length = EOF)
	FrameStderr byte = 0x04 // server→client: raw bytes (0-length = EOF)
	FrameExit   byte = 0x05 // server→client: 4-byte big-endian exit code
)

// InitPayload is the JSON body of a FrameInit frame.
type InitPayload struct {
	Argv []string `json:"argv"`
	Env  []string `json:"env"`
}

// MarshalExitCode encodes an exit code as a 4-byte big-endian payload for a FrameExit frame.
func MarshalExitCode(code int) []byte {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], uint32(code))
	return buf[:]
}

// UnmarshalExitCode decodes a 4-byte big-endian FrameExit payload into an exit code.
func UnmarshalExitCode(payload []byte) int {
	return int(binary.BigEndian.Uint32(payload))
}

// ReadFrame reads a single frame from the wire: 1-byte type + 4-byte big-endian length + payload.
func ReadFrame(r io.Reader) (byte, []byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	typ := header[0]
	length := binary.BigEndian.Uint32(header[1:])
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}
	return typ, payload, nil
}

// WriteFrame writes a single frame to the wire: 1-byte type + 4-byte big-endian length + payload.
func WriteFrame(w io.Writer, typ byte, payload []byte) error {
	var header [5]byte
	header[0] = typ
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		_, err := w.Write(payload)
		return err
	}
	return nil
}

// frameWriter wraps an io.Writer with a mutex for concurrent frame writes.
type frameWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (fw *frameWriter) writeFrame(typ byte, payload []byte) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return WriteFrame(fw.w, typ, payload)
}

// streamWriter is an io.Writer that sends each Write as a frame of the given type.
type streamWriter struct {
	fw  *frameWriter
	typ byte
}

func (sw *streamWriter) Write(p []byte) (int, error) {
	if err := sw.fw.writeFrame(sw.typ, p); err != nil {
		return 0, err
	}
	return len(p), nil
}
