package config

import (
	"encoding/json"
	"os"
	"sync"
)

type SourceType string

const (
	SourceLocal   SourceType = "Local"
	SourceWebcam  SourceType = "Web-Camera"
	SourceYouTube SourceType = "YouTube"

	DefaultConfigPath           string = "config.json"
	DefaultDetectorProcessorUrl string = "localhost:8080"
)

var SourcesList = [...]string{
	string(SourceLocal),
	string(SourceWebcam),
	string(SourceYouTube),
}

type LocalConfig struct {
	Path string `json:"path"`
}

type WebcamConfig struct {
	DeviceID string `json:"device_id"`
}

type YouTubeConfig struct {
	URL string `json:"url"`
}

type Config struct {
	mu sync.RWMutex

	ActiveSource SourceType `json:"active_source"`
	TargetFPS    uint       `json:"target_fps"`
	ScaledWitdh  int        `json:"scaled_witdh"`
	ScaledHeight int        `json:"scaled_height"`

	Local   LocalConfig   `json:"local"`
	Webcam  WebcamConfig  `json:"webcam"`
	YouTube YouTubeConfig `json:"youtube"`
}

func (c *Config) GetFPS() uint {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.TargetFPS
}

func (c *Config) SetFPS(fps uint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.TargetFPS = fps
}

func (c *Config) GetWidth() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ScaledWitdh
}

func (c *Config) SetWidth(width int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ScaledWitdh = width
}

func (c *Config) GetHeight() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ScaledHeight
}

func (c *Config) SetHeight(height int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ScaledHeight = height
}

func (c *Config) Save(path string) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)

	if err != nil {
		return
	}

	defer f.Close()

	enc := json.NewEncoder(f)
	err = enc.Encode(c)

	if err != nil {
		return
	}
}

func (c *Config) SaveByDefault() {
	c.Save(DefaultConfigPath)
}

func LoadConfigFile(path string) *Config {
	var cfg *Config = NewDefaultConfig()

	if _, err := os.Stat(path); err == nil {
		f, err := os.Open(path)

		if err != nil {
			return cfg
		}

		dec := json.NewDecoder(f)
		err = dec.Decode(cfg)

		if err != nil {
			return cfg
		}
	}

	return cfg
}

func NewDefaultConfig() *Config {
	return &Config{
		ActiveSource: SourceLocal,
		Local:        LocalConfig{Path: "..."},
		Webcam:       WebcamConfig{DeviceID: "0"},
		YouTube:      YouTubeConfig{URL: "https://..."},
		TargetFPS:    24,
		ScaledWitdh:  640,
		ScaledHeight: 640,
	}
}
