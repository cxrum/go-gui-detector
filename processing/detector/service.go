package processing

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"log"
	"net/url"
	"time"

	"vision/internal/models"

	"github.com/gorilla/websocket"
)

type RemoteDetector struct {
	serverURL string

	InputFrames  chan image.Image
	OutputResult chan []models.DetectionResult

	stopChan chan struct{}
}

func NewRemoteDetector(host string) *RemoteDetector {
	u := url.URL{Scheme: "ws", Host: host, Path: "/ws"}

	return &RemoteDetector{
		serverURL:    u.String(),
		InputFrames:  make(chan image.Image, 5),
		OutputResult: make(chan []models.DetectionResult, 5),
		stopChan:     make(chan struct{}),
	}
}

func (d *RemoteDetector) Start() {
	go d.runLoop()
}

func (d *RemoteDetector) Stop() {
	close(d.stopChan)
}

func (d *RemoteDetector) runLoop() {
	var conn *websocket.Conn
	var err error

	for {
		select {
		case <-d.stopChan:
			return
		default:
		}

		log.Println("connecting to detector server...", d.serverURL)
		conn, _, err = websocket.DefaultDialer.Dial(d.serverURL, nil)

		if err != nil {
			log.Printf("connection failed: %v. retrying in 2s...", err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Println("connected to detection server!")

		errChan := make(chan error, 1)

		go func() {
			for {
				select {
				case <-d.stopChan:
					return
				case img := <-d.InputFrames:
					var buf bytes.Buffer
					if err := jpeg.Encode(&buf, img, nil); err != nil {
						log.Println("JPEG encode error:", err)
						continue
					}

					if err := conn.WriteMessage(websocket.BinaryMessage, buf.Bytes()); err != nil {
						errChan <- err
						return
					}
				}
			}
		}()

		go func() {
			for {
				_, message, err := conn.ReadMessage()
				if err != nil {
					errChan <- err
					return
				}

				var results []models.DetectionResult
				if err := json.Unmarshal(message, &results); err != nil {
					log.Println("JSON decode error:", err)
					continue
				}

				select {
				case d.OutputResult <- results:
				default:
				}
			}
		}()

		err = <-errChan
		log.Printf("connection lost: %v", err)
		conn.Close()
	}
}
