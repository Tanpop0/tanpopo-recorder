package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Streamers []StreamerConfig `yaml:"streamers"`
}

// StreamerConfig holds configuration for a single streamer
type StreamerConfig struct {
	ScreenID string `yaml:"screen_id"`
	Schedule string `yaml:"schedule"` // Cron expression
}

// LoadConfig reads configuration from the specified file path
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
