package cosmos

import (
	"context"
	"fmt"
	"strings"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/config"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/btcsuite/btcd/btcec/v2"
)

var _ signerapp.PrivKeyRetriever = &CosmosKeyringRetriever{}

type CosmosKeyringRetriever struct {
	Kr         *ChainKeyringController
	passphrase string
}

func NewCosmosKeyringRetriever(cfg *config.CosmosKeyStoreConfig) (*CosmosKeyringRetriever, error) {
	input := strings.NewReader("")
	kr, err := CreateKeyring(cfg.KeyDirectory, cfg.ChainID, cfg.KeyringBackend, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyring: %w", err)
	}

	kc, err := NewChainKeyringControllerWithKeyring(kr, cfg.KeyName, input)
	if err != nil {
		return nil, err
	}
	return &CosmosKeyringRetriever{
		Kr:         kc,
		passphrase: cfg.Passphrase,
	}, nil
}

func (k *CosmosKeyringRetriever) PrivKey(ctx context.Context) (*btcec.PrivateKey, error) {
	privKey, err := k.Kr.GetChainPrivKey(k.passphrase)
	if err != nil {
		return nil, err
	}

	btcecPrivKey, _ := btcec.PrivKeyFromBytes(privKey.Key)

	return btcecPrivKey, nil
}
