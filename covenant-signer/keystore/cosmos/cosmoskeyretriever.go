package cosmos

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/config"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/btcsuite/btcd/btcec/v2"
)

var _ signerapp.PrivKeyRetriever = &CosmosKeyringRetriever{}

type CosmosKeyringRetriever struct {
	Kr           *ChainKeyringController
	mu           sync.Mutex
	btcecPrivKey *btcec.PrivateKey
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
		Kr:           kc,
		btcecPrivKey: nil,
	}, nil
}

func (k *CosmosKeyringRetriever) PrivKey(ctx context.Context) (*btcec.PrivateKey, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.btcecPrivKey == nil {
		return nil, fmt.Errorf("private key is not unlocked. Please call Unlock() first")
	}

	return k.btcecPrivKey, nil
}

func (k *CosmosKeyringRetriever) Unlock(ctx context.Context, passphrase string) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.btcecPrivKey != nil {
		// already unlocked
		return nil
	}

	privKey, err := k.Kr.GetChainPrivKey(passphrase)
	if err != nil {
		return fmt.Errorf("failed to unlock the key ring: %w", err)
	}

	btcecPrivKey, _ := btcec.PrivKeyFromBytes(privKey.Key)

	k.btcecPrivKey = btcecPrivKey
	return nil
}

func (k *CosmosKeyringRetriever) Lock(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.btcecPrivKey == nil {
		// already locked
		return nil
	}

	// First zero out the memory associated with the private key
	k.btcecPrivKey.Zero()
	// Clear the reference to the private key
	k.btcecPrivKey = nil

	return nil
}
