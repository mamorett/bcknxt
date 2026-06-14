package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// BackupProfile defines a single backup configuration.
type BackupProfile struct {
	Source string `json:"source"`
	Dest   string `json:"dest"`
	Tmp    string `json:"tmp"`
}

// Config is the top-level JSON structure.
type Config struct {
	Profiles map[string]BackupProfile `json:"profiles"`
}

// Default paths (used when config has no matching profile).
const (
	DefaultSource = "/data/backups"
	DefaultDest   = "bck/default"
	DefaultTmp    = "/tmp/bcknxt"
)

func loadConfig(path string) (*Config, error) {
	if path == "" {
		path = "config.json"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]BackupProfile)
	}
	return &cfg, nil
}

func getProfile(cfg *Config, name string) (*BackupProfile, error) {
	p, ok := cfg.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile %q not found in config", name)
	}
	if p.Source == "" {
		return nil, fmt.Errorf("profile %q: missing source", name)
	}
	if p.Dest == "" {
		return nil, fmt.Errorf("profile %q: missing dest", name)
	}
	if p.Tmp == "" {
		return nil, fmt.Errorf("profile %q: missing tmp", name)
	}
	return &p, nil
}
