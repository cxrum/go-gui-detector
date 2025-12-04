package ui

import (
	"fmt"
	"image"
	"image/color"
	"time"
	"vision/internal/config"
	"vision/internal/models"
	"vision/internal/ui/cwidget"
	"vision/processing/capture"
	processing "vision/processing/detector"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type DetectApp struct {
	fyneApp fyne.App
	mainWin fyne.Window

	config    *config.Config
	processor *processing.Processor

	dynamicSettings *fyne.Container
	staticSettings  *fyne.Container

	videoCanvas   *canvas.Image
	rectContainer *fyne.Container

	latencyLabel *widget.Label
	fpsLabel     *widget.Label
}

func CreateApp(p *processing.Processor, cfg *config.Config) *DetectApp {
	a := app.New()
	w := a.NewWindow("Video Stream")
	w.Resize(fyne.NewSize(1200, 600))

	return &DetectApp{
		fyneApp:   a,
		mainWin:   w,
		processor: p,
		config:    cfg,
	}
}

func (a *DetectApp) Run() {
	a.dynamicSettings = container.NewVBox()

	sourceTypeSelect := widget.NewSelect(config.SourcesList[:], func(s string) {
		a.config.ActiveSource = config.SourceType(s)
		a.refreshSettingsUI(s)
	})
	sourceTypeSelect.SetSelected(string(a.config.ActiveSource))

	settingsLabel := widget.NewLabelWithStyle("Configuration", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	a.videoCanvas = canvas.NewImageFromImage(nil)
	a.videoCanvas.FillMode = canvas.ImageFillContain
	a.videoCanvas.SetMinSize(fyne.NewSize(640, 480))

	a.rectContainer = container.NewWithoutLayout()

	videoOverlay := container.NewStack(a.videoCanvas, a.rectContainer)

	a.latencyLabel = widget.NewLabel(a.formatLatency(a.processor.Latency))
	a.fpsLabel = widget.NewLabel(a.formatFPS(a.processor.FPS))

	videoSection := container.NewBorder(
		container.NewHBox(a.fpsLabel, widget.NewSeparator(), a.latencyLabel),
		nil, nil, nil,
		videoOverlay,
	)

	a.setupConfigSettings()

	sidebar := container.NewVBox(
		settingsLabel,
		widget.NewSeparator(),
		widget.NewLabel("Source Type:"),
		sourceTypeSelect,
		widget.NewSeparator(),
		a.dynamicSettings,
		a.staticSettings,
		widget.NewSeparator(),
		widget.NewButtonWithIcon("Start Processing", theme.MediaPlayIcon(), func() {
			a.StartProcessing(true)
		}),
	)

	split := container.NewHSplit(
		container.NewPadded(sidebar),
		container.NewPadded(videoSection),
	)
	split.SetOffset(0.3)

	a.mainWin.SetContent(split)

	a.refreshSettingsUI(string(a.config.ActiveSource))

	a.mainWin.SetOnClosed(func() {
		a.config.SaveByDefault()
	})

	a.mainWin.CenterOnScreen()
	a.mainWin.ShowAndRun()
}

func (a *DetectApp) StopProcessing() {
	if a.processor.IsActive {
		a.processor.Stop()
	}

	if a.processor.IsStreamerActive {
		a.processor.InImageStream.Stop()
	}
}

func (a *DetectApp) StartProcessing(forceRestart bool) {
	if a.processor.IsActive && !forceRestart {
		return
	}

	a.StopProcessing()

	if err := a.restartStreamer(); err != nil {
		dialog.ShowError(err, a.mainWin)
		return
	}

	a.processor.Start()

	go a.runPlayerLoop()
	go a.runStatLoop()
}

func (a *DetectApp) runStatLoop() {
	uiTicker := time.NewTicker(time.Millisecond * 200)
	currentStopChan := a.processor.StopChan

	defer uiTicker.Stop()

	for {
		select {
		case <-uiTicker.C:
			fyne.Do(func() {
				a.latencyLabel.SetText(a.formatLatency(a.processor.Latency))
				a.fpsLabel.SetText(a.formatFPS(a.processor.FPS))
			})
		case <-currentStopChan:
			return
		}
	}
}

func (a *DetectApp) formatFPS(v uint64) string {
	return fmt.Sprintf("FPS: %d", v)
}

func (a *DetectApp) formatLatency(v time.Duration) string {
	return fmt.Sprintf("Latency: %d ms", v.Milliseconds())
}

func (a *DetectApp) runPlayerLoop() {
	currentStopChan := a.processor.StopChan

	frameChan := a.processor.OutImageStream
	detectionsChan := a.processor.OutDetectionsStream

	displayFPS := time.Duration(a.config.TargetFPS)
	displayTicker := time.NewTicker(time.Second / displayFPS)
	defer displayTicker.Stop()

	var lastFrame image.Image
	var lastDetections []models.DetectionResult

	for {
		select {
		case frame, ok := <-frameChan:
			if !ok {
				return
			}
			if frame != nil {
				lastFrame = frame
			}
		case detections, ok := <-detectionsChan:
			if !ok {
				return
			}
			if detections != nil {
				lastDetections = detections
			}

		case <-displayTicker.C:
			if lastFrame != nil {
				fyne.Do(func() {
					a.videoCanvas.Image = lastFrame
					a.videoCanvas.Refresh()
					a.updateRectangles(lastFrame.Bounds(), lastDetections)
				})
			}

		case <-currentStopChan:
			return
		}
	}
}

func (a *DetectApp) updateRectangles(imgBounds image.Rectangle, detections []models.DetectionResult) {
	a.rectContainer.Objects = nil

	if len(detections) == 0 {
		a.rectContainer.Refresh()
		return
	}

	canvasSize := a.videoCanvas.Size()

	scaleX := float32(canvasSize.Width) / float32(imgBounds.Dx())
	scaleY := float32(canvasSize.Height) / float32(imgBounds.Dy())

	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	offsetX := (canvasSize.Width - float32(imgBounds.Dx())*scale) / 2
	offsetY := (canvasSize.Height - float32(imgBounds.Dy())*scale) / 2

	greenColor := color.RGBA{0, 255, 0, 255}

	for _, det := range detections {
		y1 := det.Box[0]
		x1 := det.Box[1]
		y2 := det.Box[2]
		x2 := det.Box[3]

		rectX := x1*float32(imgBounds.Dx())*scale + offsetX
		rectY := y1*float32(imgBounds.Dy())*scale + offsetY
		rectW := (x2 - x1) * float32(imgBounds.Dx()) * scale
		rectH := (y2 - y1) * float32(imgBounds.Dy()) * scale

		rect := canvas.NewRectangle(color.Transparent)
		rect.StrokeColor = greenColor
		rect.StrokeWidth = 3
		rect.Resize(fyne.NewSize(rectW, rectH))
		rect.Move(fyne.NewPos(rectX, rectY))

		txt := canvas.NewText(fmt.Sprintf("%s %.0f%%", det.Class, det.Score*100), greenColor)
		txt.TextSize = 12
		txt.TextStyle = fyne.TextStyle{Bold: true}
		txt.Move(fyne.NewPos(rectX, rectY-15))

		a.rectContainer.Add(rect)
		a.rectContainer.Add(txt)
	}

	a.rectContainer.Refresh()
}

func (a *DetectApp) restartStreamer() error {
	if a.processor.InImageStream != nil {
		a.processor.InImageStream.Stop()
	}

	streamer, err := capture.NewStreamer(a.config)

	if err != nil {
		return err
	}

	a.processor.InImageStream = streamer
	return a.processor.InImageStream.Start()
}

func (a *DetectApp) setupConfigSettings() {

	a.staticSettings = container.NewVBox()

	fpsInput := cwidget.NewIntInput("FPS", "Int", int(a.config.TargetFPS), func(i int) { a.config.SetFPS(uint(i)) })
	widthInput := cwidget.NewIntInput("Width", "Int", a.config.ScaledWitdh, func(i int) { a.config.SetWidth(i) })
	heightInput := cwidget.NewIntInput("Height", "Int", a.config.ScaledHeight, func(i int) { a.config.SetHeight(i) })

	a.staticSettings.Add(fpsInput)
	a.staticSettings.Add(widthInput)
	a.staticSettings.Add(heightInput)
	a.staticSettings.Add(widget.NewButton("Save config", func() { a.StartProcessing(true) }))
}

func (a *DetectApp) refreshSettingsUI(sourceType string) {
	a.dynamicSettings.Objects = nil
	a.StopProcessing()

	switch config.SourceType(sourceType) {
	case config.SourceLocal:
		pathEntry := widget.NewEntry()
		pathEntry.SetPlaceHolder("/path/to/video.mp4")
		pathEntry.SetText(a.config.Local.Path)

		pathEntry.OnChanged = func(s string) {
			a.config.Local.Path = s
		}

		fileBtn := widget.NewButtonWithIcon("Open File", theme.FolderOpenIcon(), func() {
			dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
				if err == nil && reader != nil {
					path := reader.URI().Path()
					pathEntry.SetText(path)
				}
			}, a.mainWin)
		})

		a.dynamicSettings.Add(widget.NewLabel("Video Path:"))
		a.dynamicSettings.Add(container.NewBorder(nil, nil, nil, fileBtn, pathEntry))

	case config.SourceWebcam:
		deviceSelect := widget.NewSelect([]string{"Loading cameras..."}, func(s string) {
			if s != "Loading cameras..." && s != "No cameras found" {
				a.config.Webcam.DeviceID = s
			}
		})
		deviceSelect.SetSelected("Loading cameras...")
		deviceSelect.Disable()

		a.dynamicSettings.Add(widget.NewLabel("Select Camera:"))
		a.dynamicSettings.Add(deviceSelect)
		a.dynamicSettings.Refresh()

		go func() {
			devices, err := capture.ListCameras()

			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(err, a.mainWin)
					deviceSelect.Options = []string{"Error listing cameras"}
				} else if len(devices) == 0 {
					deviceSelect.Options = []string{"No cameras found"}
				} else {
					deviceSelect.Options = devices
					deviceSelect.Enable()

					if a.config.Webcam.DeviceID != "" {
						deviceSelect.SetSelected(a.config.Webcam.DeviceID)
					} else {
						deviceSelect.SetSelected(devices[0])
					}
				}
				deviceSelect.Refresh()
			})
		}()

	case config.SourceYouTube:

		urlEntry := widget.NewEntry()
		urlEntry.SetPlaceHolder("https://youtube.com/...")
		urlEntry.SetText(a.config.YouTube.URL)
		urlEntry.OnChanged = func(s string) {
			a.config.YouTube.URL = s
		}

		a.dynamicSettings.Add(widget.NewLabel("YouTube URL:"))
		a.dynamicSettings.Add(urlEntry)
	}

	a.dynamicSettings.Refresh()
}
