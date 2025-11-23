package capture

import (
	"image"
)

type VideoStreamer interface {
	Start() error
	Stop()
	FrameChan() <-chan image.Image
	ErrorChan() <-chan error
}
