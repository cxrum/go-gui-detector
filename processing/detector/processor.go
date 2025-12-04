package processing

import (
	"image"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"vision/internal/config"
	"vision/internal/models"
	stream "vision/processing/capture"
)

type Processor struct {
	InImageStream stream.VideoStreamer

	OutImageStream      chan image.Image
	OutDetectionsStream chan []models.DetectionResult

	ErrChan  chan error
	StopChan chan struct{}

	Latency          time.Duration
	FPS              uint64
	IsActive         bool
	IsStreamerActive bool

	cfg *config.Config
	det *RemoteDetector

	mu sync.RWMutex
}

func NewProcessor(cfg *config.Config, det *RemoteDetector) *Processor {
	return &Processor{
		cfg: cfg,
		det: det,

		ErrChan:             make(chan error, 1),
		StopChan:            make(chan struct{}),
		OutImageStream:      make(chan image.Image, cfg.GetFPS()),
		OutDetectionsStream: make(chan []models.DetectionResult, cfg.GetFPS()),
	}
}

func (p *Processor) Start() {
	p.mu.Lock()
	p.StopChan = make(chan struct{})
	p.IsActive = true
	p.mu.Unlock()

	go func() {
		p.IsActive = true
		var frameCount uint64 = 0
		lastFpsUpdate := time.Now()

		p.OutDetectionsStream = p.det.OutputResult

		for {
			select {
			case frame, ok := <-p.InImageStream.FrameChan():
				if !ok {
					p.IsActive = false
					p.IsStreamerActive = false
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

				p.mu.Lock()
				p.Latency = time.Since(start)
				p.mu.Unlock()

				select {
				case p.OutImageStream <- frame:
				default:
				}

				frameCount++
				if time.Since(lastFpsUpdate) >= time.Second {
					atomic.StoreUint64(&p.FPS, frameCount)
					frameCount = 0
					lastFpsUpdate = time.Now()
				}

			case err := <-p.InImageStream.ErrorChan():
				log.Print("Streamer error:", err)
				p.IsStreamerActive = false
				return

			case <-p.StopChan:
				p.IsActive = false
				p.IsStreamerActive = false
				return
			}
		}
	}()
}

func (p *Processor) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.IsActive {
		return
	}

	close(p.StopChan)

	p.IsActive = false
	p.IsStreamerActive = false
}
