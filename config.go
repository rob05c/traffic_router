package main

import (
	"encoding/json"
	"errors"
	"os"
)

type Config struct {
	CZFPath      string `json:"czf_path"`
	CRConfigPath string `json:"crconfig_path"`
	CRStatesPath string `json:"crstates_path"`
}

func LoadConfig(path string) (Config, error) {
	fi, err := os.Open(path)
	if err != nil {
		return Config{}, errors.New("loading file: " + err.Error())
	}
	defer fi.Close()
	cfg := Config{}
	if err := json.NewDecoder(fi).Decode(&cfg); err != nil {
		return Config{}, errors.New("decoding: " + err.Error())
	}
	return cfg, nil
}
