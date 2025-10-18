package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	MaxWasmContainers         int `json:"MaxWasmContainers"`
	MaxNativeContainers       int `json:"MaxNativeContainers"`
	RPSEpoch                  int `json:"RPSEPoch"`
	InvocationSampleThreshold int `json:"InvocationSampleThreshold"`
	ContainerCleanupInterval  int `json:"ContainerCleanupInterval"`
	ContainerExpirationTime   int `json:"ContainerExpirationTime"`
	DefaultLogLevel           int `json:"DefaultLogLevel"`
	CurrLogLevel              int `json:"CurrLogLevel"`
	UseDatabase               int `json:"UseDatabase"`
}

func CreateDefaultConfig() Config {
	var cfg Config
	cfg.MaxNativeContainers = 500
	cfg.MaxWasmContainers = 500
	cfg.RPSEpoch = 10
	cfg.InvocationSampleThreshold = 100
	cfg.ContainerCleanupInterval = 10
	cfg.ContainerExpirationTime = 60
	cfg.DefaultLogLevel = 2
	cfg.CurrLogLevel = 2
	cfg.UseDatabase = 0

	return cfg
}

func LoadConfig(filename string) (Config, error) {
	var cfg Config
	conf, err := os.ReadFile(filename)

	if err != nil {
		// Populate default config values
		cfg := CreateDefaultConfig()
		return cfg, err
	}

	err = json.Unmarshal(conf, &cfg)
	if err != nil {
		// Populate default config values
		cfg := CreateDefaultConfig()
		return cfg, err
	}

	return cfg, nil
}
