package config

import (
	"fmt"
	"net"
)

// MetricsConfig defines the server's metric configuration
type MetricsConfig struct {
	// IP of the prometheus server
	Host string `mapstructure:"host"`
	// Port of the prometheus server
	Port int `mapstructure:"port"`
}

type ParsedMetricsConfig struct {
	Host string
	Port int
}

func (cfg *MetricsConfig) Parse() (*ParsedMetricsConfig, error) {
	if cfg.Port < 1024 || cfg.Port > 65535 {
		return nil, fmt.Errorf("metrics server port must be between 1024 and 65535 (inclusive)")
	}

	ip := net.ParseIP(cfg.Host)
	if ip == nil {
		return nil, fmt.Errorf("invalid metrics server host: %v", cfg.Host)
	}

	return &ParsedMetricsConfig{
		Host: cfg.Host,
		Port: cfg.Port,
	}, nil
}

func (cfg *MetricsConfig) GetMetricsPort() int {
	return cfg.Port
}

func DefaultMetricsConfig() *MetricsConfig {
	return &MetricsConfig{
		Host: "127.0.0.1",
		Port: 2112,
	}
}
