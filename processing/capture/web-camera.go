package capture

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"os/exec"
	"regexp"
	"runtime"
	"sync"
)

type FFmpegWebcamStreamer struct {
	stopOnce sync.Once

	deviceName string
	width      int
	height     int
	targetFPS  uint

	cmd       *exec.Cmd
	frameChan chan image.Image
	errChan   chan error

	stopChan chan struct{}
}

func NewFFmpegWebcam(deviceName string, targetFps uint, scaledWidht int, scaledHeight int) *FFmpegWebcamStreamer {
	return &FFmpegWebcamStreamer{
		deviceName: deviceName,
		width:      scaledWidht,
		height:     scaledHeight,
		targetFPS:  targetFps,

		frameChan: make(chan image.Image),
		errChan:   make(chan error, 1),
		stopChan:  make(chan struct{}),
	}
}

func (ws *FFmpegWebcamStreamer) Start() error {
	var args []string

	if runtime.GOOS == "windows" {
		args = []string{
			"-f", "dshow",
			"-i", fmt.Sprintf("video=%s", ws.deviceName),
			"-vf", fmt.Sprintf("fps=%d,scale=%d:%d", ws.targetFPS, ws.width, ws.height),
			"-f", "image2pipe",
			"-pix_fmt", "rgba",
			"-vcodec", "rawvideo",
			"-",
		}
	} else {
		args = []string{
			"-f", "v4l2",
			"-i", ws.deviceName,
			"-vf", fmt.Sprintf("fps=%d,scale=%d:%d", ws.targetFPS, ws.width, ws.height),
			"-f", "image2pipe",
			"-pix_fmt", "rgba",
			"-vcodec", "rawvideo",
			"-",
		}
	}

	ws.cmd = exec.Command("ffmpeg", args...)

	var stderr bytes.Buffer
	ws.cmd.Stderr = &stderr

	stdout, err := ws.cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := ws.cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start error: %w. Details: %s", err, stderr.String())
	}

	go ws.readLoop(stdout)

	return nil
}

func (ws *FFmpegWebcamStreamer) readLoop(stdout io.ReadCloser) {
	defer close(ws.frameChan)
	defer close(ws.errChan)
	defer stdout.Close()
	defer ws.stopCmdOut()

	frameSize := ws.width * ws.height * 4
	buffer := make([]byte, frameSize)

	for {
		select {
		case <-ws.stopChan:
			return

		default:
			_, err := io.ReadFull(stdout, buffer)
			if err != nil {
				select {
				case <-ws.stopChan:
					return
				default:
					ws.errChan <- fmt.Errorf("read error: %v", err)
					return
				}
			}

			pixelData := make([]byte, len(buffer))
			copy(pixelData, buffer)

			img := &image.RGBA{
				Pix:    pixelData,
				Stride: ws.width * 4,
				Rect:   image.Rect(0, 0, ws.width, ws.height),
			}

			select {
			case ws.frameChan <- img:
			default:
			}
		}
	}
}

func (ws *FFmpegWebcamStreamer) stopCmdOut() {
	if ws.cmd != nil && ws.cmd.Process != nil {
		ws.cmd.Process.Kill()
		ws.cmd.Wait()
	}
}

func (ws *FFmpegWebcamStreamer) Stop() {
	ws.stopOnce.Do(func() {
		close(ws.stopChan)
		ws.stopCmdOut()
	})
}

func (ws *FFmpegWebcamStreamer) FrameChan() <-chan image.Image { return ws.frameChan }
func (ws *FFmpegWebcamStreamer) ErrorChan() <-chan error       { return ws.errChan }

func ListCameras() ([]string, error) {
	var cameras []string

	if runtime.GOOS == "windows" {
		cmd := exec.Command("ffmpeg", "-list_devices", "true", "-f", "dshow", "-i", "dummy")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		cmd.Run()

		output := stderr.String()
		re := regexp.MustCompile(`"([^"]+)"\s+\(video\)`)

		matches := re.FindAllStringSubmatch(output, -1)

		seen := make(map[string]bool)
		for _, m := range matches {
			name := m[1]
			if name != "dummy" && !seen[name] {
				cameras = append(cameras, name)
				seen[name] = true
			}
		}
	} else {
		cameras = []string{"/dev/video0", "/dev/video1"}
	}

	if len(cameras) == 0 {
		if runtime.GOOS == "windows" {
			return []string{"No cameras found"}, nil
		}
	}

	return cameras, nil
}
