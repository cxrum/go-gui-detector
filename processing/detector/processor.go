package processing

import (
	"fmt"
	"image"
	"image/color"
	"sync"
	"time"

	"vision/internal/config"
	"vision/internal/models"
	stream "vision/processing/capture"
)

type Processor struct {
	InImageStream  stream.VideoStreamer
	OutImageStream chan image.Image

	ErrChan  chan error
	StopChan chan struct{}

	Latency  time.Duration
	FPS      uint
	IsActive bool

	cfg *config.Config
	det *RemoteDetector

	lastResults []models.DetectionResult
	mu          sync.RWMutex
}

func NewProcessor(cfg *config.Config, det *RemoteDetector) *Processor {
	processor := Processor{
		cfg:            cfg,
		det:            det,
		ErrChan:        make(chan error, 1),
		StopChan:       make(chan struct{}),
		OutImageStream: make(chan image.Image, cfg.GetFPS()),
	}
	return &processor
}

func (p *Processor) Start() {
	go func() {
		for results := range p.det.OutputResult {
			p.mu.Lock()
			p.lastResults = results
			p.mu.Unlock()
		}
	}()

	go func() {
		p.IsActive = true
		var frameCount uint = 0
		lastFpsUpdate := time.Now()

		for {
			select {
			case frame, ok := <-p.InImageStream.FrameChan():
				if !ok {
					p.IsActive = false
					return
				}
				if frame == nil {
					continue
				}

				start := time.Now()

				select {
				case p.det.InputFrames <- frame:
				default:

				}

				if rgbaImg, ok := frame.(*image.RGBA); ok {

					p.mu.RLock()
					currentDetections := p.lastResults
					p.mu.RUnlock()

					if len(currentDetections) > 0 {
						bounds := rgbaImg.Bounds()
						imgWidth := float32(bounds.Dx())
						imgHeight := float32(bounds.Dy())

						col := color.RGBA{0, 255, 0, 255}

						for _, res := range currentDetections {

							y1 := int(res.Box[0] * imgHeight)
							x1 := int(res.Box[1] * imgWidth)
							y2 := int(res.Box[2] * imgHeight)
							x2 := int(res.Box[3] * imgWidth)

							drawRect(rgbaImg, y1, x1, y2, x2, col)
						}
					}
				}

				p.mu.Lock()
				p.Latency = time.Since(start)
				p.mu.Unlock()

				select {
				case p.OutImageStream <- frame:
				default:
				}

				frameCount++
				if time.Since(lastFpsUpdate) >= time.Second {
					p.FPS = frameCount
					frameCount = 0
					lastFpsUpdate = time.Now()
				}

			case err := <-p.InImageStream.ErrorChan():
				fmt.Println("Помилка стрімера:", err)
				p.IsActive = false
				return

			case <-p.StopChan:
				p.IsActive = false
				return
			}
		}
	}()
}

func (p *Processor) Stop() {
	p.StopChan <- struct{}{}
}

func drawRect(img *image.RGBA, y1, x1, y2, x2 int, col color.Color) {
	thickness := 3
	bounds := img.Bounds()

	setPixel := func(x, y int) {
		if x >= bounds.Min.X && x < bounds.Max.X && y >= bounds.Min.Y && y < bounds.Max.Y {
			img.Set(x, y, col)
		}
	}

	for t := 0; t < thickness; t++ {
		for x := x1; x <= x2; x++ {
			setPixel(x, y1+t)
			setPixel(x, y2-t)
		}
		for y := y1; y <= y2; y++ {
			setPixel(x1+t, y)
			setPixel(x2-t, y)
		}
	}
}
