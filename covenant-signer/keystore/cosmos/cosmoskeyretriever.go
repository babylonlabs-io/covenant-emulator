package cosmos

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/config"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/btcsuite/btcd/btcec/v2"
)

var _ signerapp.PrivKeyRetriever = &KeyringRetriever{}

type KeyringRetriever struct {
	Kr           *ChainKeyringController
	mu           sync.Mutex
	btcecPrivKey *btcec.PrivateKey
}

func NewCosmosKeyringRetriever(cfg *config.CosmosKeyStoreConfig) (*KeyringRetriever, error) {
	input := strings.NewReader("")
	kr, err := CreateKeyring(cfg.KeyDirectory, cfg.ChainID, cfg.KeyringBackend, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyring: %w", err)
	}

	kc, err := NewChainKeyringControllerWithKeyring(kr, cfg.KeyName, input)
	if err != nil {
		return nil, err
	}

	return &KeyringRetriever{
		Kr:           kc,
		btcecPrivKey: nil,
	}, nil
}

func (k *KeyringRetriever) PrivKey(_ context.Context) (*btcec.PrivateKey, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.btcecPrivKey == nil {
		return nil, errors.New("private key is not unlocked. Please call Unlock() first")
	}

	return k.btcecPrivKey, nil
}

func (k *KeyringRetriever) Unlock(_ context.Context, passphrase string) error {
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

func (k *KeyringRetriever) Lock(_ context.Context) error {
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
