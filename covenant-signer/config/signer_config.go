package config

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg"
)

type SignerType int

const (
	PsbtSigner SignerType = iota
	PrivKeySigner
)

func SignerFromString(s string) (SignerType, error) {
	switch s {
	case "psbt":
		return PsbtSigner, nil
	case "privkey":
		return PrivKeySigner, nil
	default:
		return -1, fmt.Errorf("unknown signer type %s", s)
	}
}

type BtcSignerConfig struct {
	Host       string `mapstructure:"host"`
	User       string `mapstructure:"user"`
	Pass       string `mapstructure:"pass"`
	Network    string `mapstructure:"network"`
	SignerType string `mapstructure:"signer-type"`
}

type ParsedBtcSignerConfig struct {
	Host       string
	User       string
	Pass       string
	Network    *chaincfg.Params
	SignerType SignerType
}

func DefaultBtcSignerConfig() *BtcSignerConfig {
	return &BtcSignerConfig{
		Host:       "localhost:18556",
		User:       "user",
		Pass:       "pass",
		Network:    "regtest",
		SignerType: "psbt",
	}
}

func (c *ParsedBtcSignerConfig) ToBtcConfig() *ParsedBtcConfig {
	return &ParsedBtcConfig{
		Host:    c.Host,
		User:    c.User,
		Pass:    c.Pass,
		Network: c.Network,
	}
}

func (c *BtcSignerConfig) Parse() (*ParsedBtcSignerConfig, error) {
	params, err := c.getBtcNetworkParams()

	if err != nil {
		return nil, err
	}

	signerType, err := SignerFromString(c.SignerType)

	if err != nil {
		return nil, err
	}

	return &ParsedBtcSignerConfig{
		Host:       c.Host,
		User:       c.User,
		Pass:       c.Pass,
		Network:    params,
		SignerType: signerType,
	}, nil
}

func (cfg *BtcSignerConfig) getBtcNetworkParams() (*chaincfg.Params, error) {
	switch cfg.Network {
	case "testnet3":
		return &chaincfg.TestNet3Params, nil
	case "mainnet":
		return &chaincfg.MainNetParams, nil
	case "regtest":
		return &chaincfg.RegressionNetParams, nil
	case "simnet":
		return &chaincfg.SimNetParams, nil
	case "signet":
		return &chaincfg.SigNetParams, nil
	default:
		return nil, fmt.Errorf("unknown network %s", cfg.Network)
	}
}
