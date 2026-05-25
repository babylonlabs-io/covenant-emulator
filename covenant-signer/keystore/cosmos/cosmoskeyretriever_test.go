package cosmos_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/config"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/keystore/cosmos"
	"github.com/stretchr/testify/require"
)

// TestPrivKeyConcurrentLockNoRace ensures that a key returned by PrivKey() is an
// isolated copy: a concurrent Lock() that zeroes the stored key must not corrupt
// the key a signer is still using. Run with -race to catch regressions.
func TestPrivKeyConcurrentLockNoRace(t *testing.T) {
	ctx := context.Background()

	const (
		chainID    = "test-chain"
		keyName    = "test-covenant-key"
		backend    = "test"
		passphrase = ""
		hdPath     = "m/44'/0'/0'/0/0"
	)

	keyDir := t.TempDir()
	_, err := cosmos.CreateCovenantKey(keyDir, chainID, keyName, backend, passphrase, hdPath)
	require.NoError(t, err)

	retriever, err := cosmos.NewCosmosKeyringRetriever(&config.CosmosKeyStoreConfig{
		ChainID:        chainID,
		KeyDirectory:   keyDir,
		KeyringBackend: backend,
		KeyName:        keyName,
	})
	require.NoError(t, err)

	require.NoError(t, retriever.Unlock(ctx, passphrase))

	privKey, err := retriever.PrivKey(ctx)
	require.NoError(t, err)
	pubKeyBefore := privKey.PubKey().SerializeCompressed()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		pk, pkErr := retriever.PrivKey(ctx)
		require.NoError(t, pkErr)
		// Hold the copy across a window where Lock() runs, then read from it.
		time.Sleep(10 * time.Millisecond)
		require.Equal(t, pubKeyBefore, pk.PubKey().SerializeCompressed(),
			"copy returned by PrivKey() must not be affected by a concurrent Lock()")
	}()

	go func() {
		defer wg.Done()
		time.Sleep(1 * time.Millisecond)
		require.NoError(t, retriever.Lock(ctx))
	}()

	wg.Wait()

	// After Lock(), the retriever has no key and must report it.
	_, err = retriever.PrivKey(ctx)
	require.ErrorContains(t, err, "not unlocked")
}
