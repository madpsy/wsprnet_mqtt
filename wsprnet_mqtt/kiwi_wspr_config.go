package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// KiwiWSPRConfig represents the kiwi_wspr application configuration
type KiwiWSPRConfig struct {
	MQTT          KiwiMQTTConfig    `yaml:"mqtt"`
	KiwiInstances []KiwiInstance    `yaml:"kiwi_instances"`
	WSPRBands     []KiwiWSPRBand    `yaml:"wspr_bands"`
	Decoder       KiwiDecoderConfig `yaml:"decoder"`
	Logging       KiwiLoggingConfig `yaml:"logging"`
}

// KiwiMQTTConfig holds MQTT broker configuration from kiwi_wspr
type KiwiMQTTConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	UseTLS      bool   `yaml:"use_tls"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	TopicPrefix string `yaml:"topic_prefix"`
	QoS         byte   `yaml:"qos"`
	Retain      bool   `yaml:"retain"`
}

// KiwiInstance represents a KiwiSDR instance from kiwi_wspr config
type KiwiInstance struct {
	Name            string `yaml:"name"`
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	Password        string `yaml:"password"`
	User            string `yaml:"user"`
	Enabled         bool   `yaml:"enabled"`
	MQTTTopicPrefix string `yaml:"mqtt_topic_prefix"` // Optional: Override global MQTT topic prefix for this instance
}

// KiwiWSPRBand represents a WSPR band configuration from kiwi_wspr
type KiwiWSPRBand struct {
	Name      string  `yaml:"name"`
	Frequency float64 `yaml:"frequency"`
	Instance  string  `yaml:"instance"`
	Enabled   bool    `yaml:"enabled"`
}

// KiwiDecoderConfig holds decoder settings from kiwi_wspr
type KiwiDecoderConfig struct {
	WSPRDPath   string `yaml:"wsprd_path"`
	WorkDir     string `yaml:"work_dir"`
	KeepWav     bool   `yaml:"keep_wav"`
	Compression bool   `yaml:"compression"`
}

// KiwiLoggingConfig holds logging settings from kiwi_wspr
type KiwiLoggingConfig struct {
	Level string `yaml:"level"`
	Quiet bool   `yaml:"quiet"`
}

// LoadKiwiWSPRConfig loads kiwi_wspr configuration from a YAML file
func LoadKiwiWSPRConfig(filename string) (*KiwiWSPRConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read kiwi_wspr config file: %w", err)
	}

	var config KiwiWSPRConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse kiwi_wspr config file: %w", err)
	}

	return &config, nil
}

// GetEnabledBands returns all enabled WSPR bands from kiwi_wspr config
func (c *KiwiWSPRConfig) GetEnabledBands() []KiwiWSPRBand {
	var enabled []KiwiWSPRBand
	for _, band := range c.WSPRBands {
		if band.Enabled {
			enabled = append(enabled, band)
		}
	}
	return enabled
}

// GetInstance returns a KiwiSDR instance by name from kiwi_wspr config
func (c *KiwiWSPRConfig) GetInstance(name string) *KiwiInstance {
	for i := range c.KiwiInstances {
		if c.KiwiInstances[i].Name == name {
			return &c.KiwiInstances[i]
		}
	}
	return nil
}

// GetMQTTTopicPrefix returns the MQTT topic prefix for kiwi_wspr
// If an instance has a custom prefix, it returns that, otherwise the global prefix
func (c *KiwiWSPRConfig) GetMQTTTopicPrefix(instanceName string) string {
	inst := c.GetInstance(instanceName)
	if inst != nil && inst.MQTTTopicPrefix != "" {
		return inst.MQTTTopicPrefix
	}
	return c.MQTT.TopicPrefix
}
