package config

import "fmt"

type KeyStoreType int

const (
	CosmosKeyStore KeyStoreType = iota
)

func KeyStoreToString(c KeyStoreType) (string, error) {
	switch c {
	case CosmosKeyStore:
		return "cosmos", nil
	default:
		return "", fmt.Errorf("unknown key store type")
	}
}

func KeyStoreFromString(s string) (KeyStoreType, error) {
	switch s {
	case "cosmos":
		return CosmosKeyStore, nil
	default:
		return -1, fmt.Errorf("unknown key store type")
	}
}

type CosmosKeyStoreConfig struct {
	ChainID        string `mapstructure:"chain-id"`
	KeyDirectory   string `mapstructure:"key-directory"`
	KeyringBackend string `mapstructure:"keyring-backend"`
	KeyName        string `mapstructure:"key-name"`
}

type KeyStoreConfig struct {
	KeyStoreType   string                `mapstructure:"keystore-type"`
	CosmosKeyStore *CosmosKeyStoreConfig `mapstructure:"cosmos"`
}

func DefaultKeyStoreConfig() *KeyStoreConfig {
	defaultKeyStoreType, err := KeyStoreToString(CosmosKeyStore)
	if err != nil {
		panic(err)
	}

	return &KeyStoreConfig{
		KeyStoreType:   defaultKeyStoreType,
		CosmosKeyStore: &CosmosKeyStoreConfig{},
	}
}

type ParsedKeyStoreConfig struct {
	KeyStoreType   KeyStoreType
	CosmosKeyStore *CosmosKeyStoreConfig
}

func (cfg *KeyStoreConfig) Parse() (*ParsedKeyStoreConfig, error) {
	keyStoreType, err := KeyStoreFromString(cfg.KeyStoreType)
	if err != nil {
		return nil, err
	}

	return &ParsedKeyStoreConfig{
		KeyStoreType:   keyStoreType,
		CosmosKeyStore: cfg.CosmosKeyStore,
	}, nil
}
