package execstream

import (
	"encoding/json"
	"io"
)

type FrameType string

const (
	FrameTypeCommand FrameType = "command"
	FrameTypeStdin   FrameType = "stdin"
	FrameTypeStdout  FrameType = "stdout"
	FrameTypeStderr  FrameType = "stderr"
	FrameTypeResize  FrameType = "resize"
	FrameTypeExit    FrameType = "exit"
	FrameTypeError   FrameType = "error"
)

// Frame captures a single event flowing between controller, worker and clients.
//
// The payload is encoded as JSON where binary blobs (stdin/stdout/stderr data) are
// automatically base64-encoded by the JSON encoder.
type Frame struct {
	Type FrameType `json:"type"`

	Command  *Command      `json:"command,omitempty"`
	Data     []byte        `json:"data,omitempty"`
	Terminal *TerminalSize `json:"terminal,omitempty"`
	Exit     *Exit         `json:"exit,omitempty"`
	Error    string        `json:"error,omitempty"`
}

type Command struct {
	Name        string        `json:"name"`
	Args        []string      `json:"args,omitempty"`
	Interactive bool          `json:"interactive,omitempty"`
	TTY         bool          `json:"tty,omitempty"`
	Terminal    *TerminalSize `json:"terminal,omitempty"`
}

type TerminalSize struct {
	Rows uint32 `json:"rows"`
	Cols uint32 `json:"cols"`
}

type Exit struct {
	Code int32 `json:"code"`
}

func NewEncoder(w io.Writer) *json.Encoder {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)

	return encoder
}

func NewDecoder(r io.Reader) *json.Decoder {
	return json.NewDecoder(r)
}

func WriteFrame(encoder *json.Encoder, frame *Frame) error {
	return encoder.Encode(frame)
}

func ReadFrame(decoder *json.Decoder, frame *Frame) error {
	return decoder.Decode(frame)
}
