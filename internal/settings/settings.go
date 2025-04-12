package settings

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Bot struct {
		ID string `yaml:"id"`
	} `yaml:"bot"`
}

func LoadConfig() (*Config, error) {
	path := filepath.Join(".", "settings.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
