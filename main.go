package main

import (
	"vision/internal/config"
	ui "vision/internal/ui"
	processing "vision/processing/detector"
)

func main() {
	det := processing.NewRemoteDetector("localhost:8080")

	det.Start()
	defer det.Stop()

	cfg := config.LoadConfigFile(config.DefaultConfigPath)
	proc := processing.NewProcessor(cfg, det)

	app := ui.CreateApp(proc, cfg)

	app.Run()
}
