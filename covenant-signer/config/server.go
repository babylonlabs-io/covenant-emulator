package config

import "time"

type ServerConfig struct {
	Host             string `mapstructure:"host"`
	Port             int    `mapstructure:"port"`
	WriteTimeout     uint32 `mapstructure:"write-timeout"`
	ReadTimeout      uint32 `mapstructure:"read-timeout"`
	IdleTimeout      uint32 `mapstructure:"idle-timeout"`
	MaxContentLength uint32 `mapstructure:"max-content-length"`
	HMACKey          string `mapstructure:"hmac-key"`
}

type ParsedServerConfig struct {
	Host             string
	Port             int
	WriteTimeout     time.Duration
	ReadTimeout      time.Duration
	IdleTimeout      time.Duration
	MaxContentLength uint32
	HMACKey          string
}

func (c *ServerConfig) Parse() (*ParsedServerConfig, error) {
	// TODO Add some validations
	return &ParsedServerConfig{
		Host:             c.Host,
		Port:             c.Port,
		WriteTimeout:     time.Duration(c.WriteTimeout) * time.Second,
		ReadTimeout:      time.Duration(c.ReadTimeout) * time.Second,
		IdleTimeout:      time.Duration(c.IdleTimeout) * time.Second,
		MaxContentLength: c.MaxContentLength,
		HMACKey:          c.HMACKey,
	}, nil
}

func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Host:             "127.0.0.1",
		Port:             9791,
		WriteTimeout:     15,
		ReadTimeout:      15,
		IdleTimeout:      120,
		MaxContentLength: 10 * 1024 * 1024, // 10MB,
		HMACKey:          "",
	}
}
