package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	RomsDir string `json:"roms_dir"`
}

func getConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "gosms_config.json"
	}
	return filepath.Join(filepath.Dir(exe), "gosms_config.json")
}

func loadConfig() Config {
	return loadConfigFrom(getConfigPath())
}

func loadConfigFrom(path string) Config {
	cfg := Config{RomsDir: "roms"}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.RomsDir == "" {
		return Config{RomsDir: "roms"}
	}
	return cfg
}

func saveConfig(cfg Config) {
	_ = saveConfigTo(getConfigPath(), cfg)
}

func saveConfigTo(path string, cfg Config) error {
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(path, data, 0644)
}
