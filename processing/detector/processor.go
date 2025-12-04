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

	pendingTimestamps chan time.Time
	mu                sync.RWMutex
}

func NewProcessor(cfg *config.Config, det *RemoteDetector) *Processor {
	return &Processor{
		cfg:                 cfg,
		det:                 det,
		ErrChan:             make(chan error, 1),
		StopChan:            make(chan struct{}),
		OutImageStream:      make(chan image.Image, cfg.GetFPS()),
		OutDetectionsStream: make(chan []models.DetectionResult, cfg.GetFPS()),
		pendingTimestamps:   make(chan time.Time, cfg.GetFPS()*2),
	}
}

func (p *Processor) Start() {
	p.mu.Lock()
	p.StopChan = make(chan struct{})
	p.IsActive = true
	p.mu.Unlock()

	go p.processResultsLoop()

	go func() {
		p.IsActive = true
		var frameCount uint64 = 0
		lastFpsUpdate := time.Now()

		for {
			select {
			case frame, ok := <-p.InImageStream.FrameChan():
				if !ok {
					p.Stop()
					return
				}
				if frame == nil {
					continue
				}

				select {
				case p.det.InputFrames <- frame:
					select {
					case p.pendingTimestamps <- time.Now():
					default:
					}
				default:
				}

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
				p.Stop()
				return

			case <-p.StopChan:
				return
			}
		}
	}()
}

func (p *Processor) processResultsLoop() {
	for {
		select {
		case results, ok := <-p.det.OutputResult:
			if !ok {
				return
			}

			var startTime time.Time
			select {
			case startTime = <-p.pendingTimestamps:
				p.mu.Lock()
				p.Latency = time.Since(startTime)
				p.mu.Unlock()
			default:
			}

			select {
			case p.OutDetectionsStream <- results:
			default:
			}

		case <-p.StopChan:
			return
		}
	}
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
