package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"strings"

	"github.com/OliverSchlueter/goutils/idgen"
	"github.com/OliverSchlueter/sco-protocol/pkg/sharedmodels"
)

type Config struct {
	LogLevel    string                  `json:"log_level"`
	AccessToken string                  `json:"access_token"`
	Tasks       []sharedmodels.NodeTask `json:"tasks"`
}

func (c *Config) SlogLevel() slog.Level {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = "config.json"
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return generateDefaultConfig(path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func generateDefaultConfig(path string) (*Config, error) {
	defaultConfig := Config{
		LogLevel:    "info",
		AccessToken: idgen.GenerateID(64),
		Tasks: []sharedmodels.NodeTask{
			{
				Node:                 "agent-1",
				ContainerName:        "sco-gitea",
				Image:                "docker.gitea.com/gitea:latest",
				Command:              []string{},
				EnvironmentVariables: []string{},
				ExposedPorts: map[string]string{
					"3000": "3000",
					"22":   "2222",
				},
				Volumes:   []string{},
				MaxCPU:    0.5,
				MaxMemory: 512,
			},
		},
	}

	data, err := json.MarshalIndent(defaultConfig, "", "  ")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return nil, err
	}

	return &defaultConfig, nil
}
