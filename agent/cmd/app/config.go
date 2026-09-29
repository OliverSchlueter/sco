package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	LogLevel    string `json:"log_level"`
	NodeName    string `json:"node_name"`
	Endpoint    string `json:"endpoint"`
	AccessToken string `json:"access_token"`
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
		NodeName:    "agent-1",
		Endpoint:    "localhost:8080",
		AccessToken: "PASTE_YOUR_ACCESS_TOKEN_HERE",
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
