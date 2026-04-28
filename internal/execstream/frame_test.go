package execstream

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFrameRoundTripsWatermark(t *testing.T) {
	frame := Frame{
		Type:      FrameTypeHistory,
		Watermark: 42,
	}

	payload, err := json.Marshal(frame)
	require.NoError(t, err)

	var decoded Frame
	err = json.Unmarshal(payload, &decoded)
	require.NoError(t, err)
	require.Equal(t, frame, decoded)
}
