//go:build e2e
// +build e2e

package e2etest

import (
	"context"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"

	"github.com/babylonlabs-io/babylon/btcstaking"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/config"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/itest/containers"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/keystore/cosmos"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/observability/metrics"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"
)

var (
	netParams = &chaincfg.RegressionNetParams
)

type TestManager struct {
	t               *testing.T
	bitcoindHandler *BitcoindTestHandler
	walletPass      string
	signerConfig    *config.Config
	app             *signerapp.SignerApp
	server          *signerservice.SigningServer
}

func StartManager(
	t *testing.T,
	numMatureOutputsInWallet uint32,
	useEncryptedFileKeyRing bool,
) *TestManager {
	m, err := containers.NewManager()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = m.ClearResources()
	})

	h := NewBitcoindHandler(t, m)
	h.Start()

	passphrase := "pass"
	_ = h.CreateWallet("test-wallet", passphrase)
	// only outputs which are 100 deep are mature
	_ = h.GenerateBlocks(int(numMatureOutputsInWallet) + 100)

	appConfig := config.DefaultConfig()

	var retriever *cosmos.CosmosKeyringRetriever

	if useEncryptedFileKeyRing {
		appConfig.KeyStore.KeyStoreType = "cosmos"
		// ChainID does not influence keyring with key imported from hex string
		appConfig.KeyStore.CosmosKeyStore.ChainID = ""
		appConfig.KeyStore.CosmosKeyStore.KeyName = "test"
		appConfig.KeyStore.CosmosKeyStore.KeyDirectory = "./testkeyring"
		appConfig.KeyStore.CosmosKeyStore.KeyringBackend = "file"

		retriever, err = cosmos.NewCosmosKeyringRetriever(appConfig.KeyStore.CosmosKeyStore)
		require.NoError(t, err)
	} else {
		appConfig.KeyStore.KeyStoreType = "cosmos"
		appConfig.KeyStore.CosmosKeyStore.ChainID = "test-chain"
		appConfig.KeyStore.CosmosKeyStore.KeyName = "test-key"
		appConfig.KeyStore.CosmosKeyStore.KeyDirectory = ""
		appConfig.KeyStore.CosmosKeyStore.KeyringBackend = "memory"

		retriever, err = cosmos.NewCosmosKeyringRetriever(appConfig.KeyStore.CosmosKeyStore)
		require.NoError(t, err)

		covPrivKey, err := btcec.NewPrivateKey()
		require.NoError(t, err)

		hexPrivKey := hex.EncodeToString(covPrivKey.Serialize())

		// Import private key to keyring, from hex string
		err = retriever.Kr.GetKeyring().ImportPrivKeyHex(
			appConfig.KeyStore.CosmosKeyStore.KeyName,
			hexPrivKey,
			"secp256k1",
		)
		require.NoError(t, err)
	}

	app := signerapp.NewSignerApp(
		retriever,
	)

	met := metrics.NewCovenantSignerMetrics()
	parsedConfig, err := appConfig.Parse()
	require.NoError(t, err)

	server, err := signerservice.New(
		context.Background(),
		parsedConfig,
		app,
		met,
	)

	require.NoError(t, err)

	go func() {
		_ = server.Start()
	}()

	// Give some time to launch server
	time.Sleep(3 * time.Second)

	t.Cleanup(func() {
		_ = server.Stop(context.TODO())
	})

	return &TestManager{
		t:               t,
		bitcoindHandler: h,
		walletPass:      passphrase,
		signerConfig:    appConfig,
		app:             app,
		server:          server,
	}
}

func (tm *TestManager) SigningServerUrl() string {
	return fmt.Sprintf("http://%s:%d", tm.signerConfig.Server.Host, tm.signerConfig.Server.Port)
}

func (tm *TestManager) verifyResponse(
	resp *signerapp.ParsedSigningResponse,
	req *signerapp.ParsedSigningRequest,
	covenantPubKey *btcec.PublicKey,
) error {

	slashAdaptorSig, err := asig.NewAdaptorSignatureFromBytes(resp.SlashAdaptorSigs[0])

	if err != nil {
		return err
	}

	err = btcstaking.EncVerifyTransactionSigWithOutput(
		req.SlashingTx,
		req.StakingTx.TxOut[req.StakingOutputIdx],
		req.SlashingScript,
		covenantPubKey,
		req.FpEncKeys[0],
		slashAdaptorSig,
	)

	if err != nil {
		return fmt.Errorf("failed to verify slash adaptor signature for slashing tx: %w", err)
	}

	slashUnbondingAdaptorSig, err := asig.NewAdaptorSignatureFromBytes(resp.SlashUnbondingAdaptorSigs[0])

	if err != nil {
		return err
	}

	err = btcstaking.EncVerifyTransactionSigWithOutput(
		req.SlashUnbondingTx,
		req.UnbondingTx.TxOut[0],
		req.UnbondingSlashingScript,
		covenantPubKey,
		req.FpEncKeys[0],
		slashUnbondingAdaptorSig,
	)

	if err != nil {
		return fmt.Errorf("failed to verify slash unbonding adaptor signature for slash unbonding tx: %w", err)
	}

	err = btcstaking.VerifyTransactionSigWithOutput(
		req.UnbondingTx,
		req.StakingTx.TxOut[req.StakingOutputIdx],
		req.UnbondingScript,
		covenantPubKey,
		resp.UnbondingSig.Serialize(),
	)

	if err != nil {
		return fmt.Errorf("failed to verify unbonding signature for unbonding tx: %w", err)
	}

	return nil
}
