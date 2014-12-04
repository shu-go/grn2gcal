package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
)

type Config struct {
	Garoon GaroonConfig `json:"garoon"`
	Gcal   GcalConfig   `json:"gcal"`
}

type GaroonConfig struct {
	// url of grn.exe
	BaseUrl string `json:"url"`

	// account name.
	Account string `json:"account"`

	// (optional) password.
	Password string `json:"password"`
}

type GcalConfig struct {
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func NewConfig(filename string) (*Config, error) {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	if err = json.Unmarshal(file, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func CreateConfigTemplate(filename string) error {
	var config Config
	marshaled, err := json.Marshal(config)
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(filename, marshaled, 0600); err != nil {
		return err
	}

	return nil
}

func ValidateConfig(config *Config) error {
	if config.Gcal.ClientId == "" {
		return errors.New("config validattion error: gcal.client_id is missing")
	}
	if config.Gcal.ClientSecret == "" {
		return errors.New("config validattion error: gcal.client_secret is missing")
	}

	return nil
}
