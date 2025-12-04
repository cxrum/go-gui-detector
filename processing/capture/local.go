package capture

import (
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os/exec"
	"sync"
	"time"
)

type LocalFileStreamer struct {
	stopOnce sync.Once

	path      string
	targetFPS uint

	s_width  int
	s_height int

	r_width  uint16
	r_height uint16

	cmd       *exec.Cmd
	frameChan chan image.Image
	errChan   chan error
	stopChan  chan struct{}
}

func NewLocalStreamer(path string, targetFPS uint, scaledWidht int, scaledHeight int) (*LocalFileStreamer, error) {
	w, h, err := probeVideoDimensions(path)
	if err != nil {
		return nil, fmt.Errorf("failed to probe video: %w", err)
	}

	return &LocalFileStreamer{
		path:      path,
		targetFPS: targetFPS,
		r_width:   w,
		r_height:  h,
		s_width:   scaledWidht,
		s_height:  scaledHeight,
		frameChan: make(chan image.Image, 10),
		errChan:   make(chan error, 1),
		stopChan:  make(chan struct{}),
	}, nil
}

func (ls *LocalFileStreamer) Start() error {

	args := []string{
		"-i", ls.path,
		"-vf", fmt.Sprintf("fps=%d,scale=%d:%d:flags=neighbor", ls.targetFPS, ls.s_width, ls.s_height),
		"-f", "image2pipe",
		"-pix_fmt", "rgba",
		"-vcodec", "rawvideo",
		"-",
	}

	ls.cmd = exec.Command("ffmpeg", args...)

	stdout, err := ls.cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := ls.cmd.Start(); err != nil {
		return err
	}

	go ls.readFrames(stdout)

	return nil
}

const (
	bytePerFrame uint16 = 4
	standartFps  uint   = 30
)

func (ls *LocalFileStreamer) readFrames(stdout io.ReadCloser) {
	defer close(ls.frameChan)
	defer close(ls.errChan)
	defer stdout.Close()
	defer ls.stopCmdOut()

	width := int(ls.s_width)
	height := int(ls.s_height)
	bpf := int(bytePerFrame)
	frameSize := width * height * bpf
	buffer := make([]byte, frameSize)

	if ls.targetFPS == 0 {
		ls.targetFPS = standartFps
	}

	frameDuration := time.Second / time.Duration(ls.targetFPS)
	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ls.stopChan:
			return

		case <-ticker.C:
			_, err := io.ReadFull(stdout, buffer)
			if err != nil {
				select {
				case <-ls.stopChan:
					return
				default:
					ls.errChan <- fmt.Errorf("read error: %v", err)
					return
				}
			}

			pixelData := make([]byte, len(buffer))
			copy(pixelData, buffer)

			img := &image.RGBA{
				Pix:    pixelData,
				Stride: width * bpf,
				Rect:   image.Rect(0, 0, width, height),
			}

			select {
			case ls.frameChan <- img:
			case <-ls.stopChan:
				return
			}
		}
	}
}

func (ls *LocalFileStreamer) stopCmdOut() {
	if ls.cmd != nil && ls.cmd.Process != nil {
		ls.cmd.Process.Kill()
		ls.cmd.Wait()
	}
}

func (ls *LocalFileStreamer) Stop() {
	ls.stopOnce.Do(func() {
		close(ls.stopChan)
		ls.stopCmdOut()
	})
}

func (ls *LocalFileStreamer) FrameChan() <-chan image.Image {
	return ls.frameChan
}

func (ls *LocalFileStreamer) ErrorChan() <-chan error {
	return ls.errChan
}

type probeData struct {
	Streams []struct {
		Width  uint16 `json:"width"`
		Height uint16 `json:"height"`
	} `json:"streams"`
}

func probeVideoDimensions(path string) (uint16, uint16, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "json",
		path,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, 0, err
	}

	var data probeData
	if err := json.Unmarshal(output, &data); err != nil {
		return 0, 0, err
	}

	if len(data.Streams) == 0 {
		return 0, 0, fmt.Errorf("no video streams found")
	}

	return data.Streams[0].Width, data.Streams[0].Height, nil
}
