package execstream

import (
	"context"
	"encoding/json"

	"github.com/coder/websocket"
)

type FrameType string

const (
	FrameTypeStdin         FrameType = "stdin"
	FrameTypeStdout        FrameType = "stdout"
	FrameTypeStderr        FrameType = "stderr"
	FrameTypeExit          FrameType = "exit"
	FrameTypeError         FrameType = "error"
	FrameTypeHistory       FrameType = "history"
	FrameTypeNoMoreHistory FrameType = "no_more_history"
	FrameTypeAck           FrameType = "ack"
	FrameTypeDetach        FrameType = "detach"
	FrameTypeClose         FrameType = "close"
)

type Frame struct {
	Type      FrameType `json:"type"`
	Data      []byte    `json:"data,omitempty"`
	Exit      *Exit     `json:"exit,omitempty"`
	Error     string    `json:"error,omitempty"`
	Watermark uint64    `json:"watermark,omitempty"`
}

type Exit struct {
	Code int32 `json:"code"`
}

func WriteFrame(ctx context.Context, wsConn *websocket.Conn, frame *Frame) error {
	frameBytes, err := json.Marshal(frame)
	if err != nil {
		return err
	}

	return wsConn.Write(ctx, websocket.MessageText, frameBytes)
}
