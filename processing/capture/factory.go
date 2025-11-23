package capture

import (
	"fmt"

	config "vision/internal/config"
)

func NewStreamer(t *config.Config) (VideoStreamer, error) {
	switch t.ActiveSource {
	case config.SourceWebcam:
		return NewFFmpegWebcam(t.Webcam.DeviceID, t.GetFPS(), t.GetWidth(), t.GetHeight()), nil
	case config.SourceLocal:
		return NewLocalStreamer(t.Local.Path, t.GetFPS(), t.GetWidth(), t.GetHeight())
	default:
		return nil, fmt.Errorf("невідоме джерело: %s", t.ActiveSource)
	}
}
