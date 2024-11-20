//go:build e2e
// +build e2e

package e2etest

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"testing"
	"time"

	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"

	sdkmath "cosmossdk.io/math"
	"github.com/babylonlabs-io/babylon/btcstaking"
	staking "github.com/babylonlabs-io/babylon/btcstaking"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/config"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/itest/containers"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/observability/metrics"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/types"
)

var (
	netParams              = &chaincfg.RegressionNetParams
	eventuallyPollInterval = 100 * time.Millisecond
	eventuallyTimeout      = 10 * time.Second
)

type TestManager struct {
	t               *testing.T
	bitcoindHandler *BitcoindTestHandler
	walletPass      string
	covenantPrivKey *btcec.PrivateKey
	signerConfig    *config.Config
	app             *signerapp.SignerApp
	server          *signerservice.SigningServer
}

type stakingData struct {
	stakingAmount  btcutil.Amount
	stakingTime    uint16
	stakingFeeRate btcutil.Amount
}

func defaultStakingData() *stakingData {
	return &stakingData{
		stakingAmount:  btcutil.Amount(100000),
		stakingTime:    10000,
		stakingFeeRate: btcutil.Amount(5000), // feeRatePerKb
	}
}

func StartManager(
	t *testing.T,
	numMatureOutputsInWallet uint32) *TestManager {
	m, err := containers.NewManager()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = m.ClearResources()
	})

	h := NewBitcoindHandler(t, m)
	h.Start()

	// Give some time to launch and bitcoind
	time.Sleep(2 * time.Second)

	passphrase := "pass"
	_ = h.CreateWallet("test-wallet", passphrase)
	// only outputs which are 100 deep are mature
	_ = h.GenerateBlocks(int(numMatureOutputsInWallet) + 100)

	appConfig := config.DefaultConfig()

	covenantPrivateKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	privKeyRetriever := signerapp.NewHardcodedPrivKeyRetriever(covenantPrivateKey)

	app := signerapp.NewSignerApp(
		privKeyRetriever,
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
		covenantPrivKey: covenantPrivateKey,
		signerConfig:    appConfig,
		app:             app,
		server:          server,
	}
}

func (tm *TestManager) SigningServerUrl() string {
	return fmt.Sprintf("http://%s:%d", tm.signerConfig.Server.Host, tm.signerConfig.Server.Port)
}

func buildDataToSign(t *testing.T, covnenantPublicKey *btcec.PublicKey) signerapp.ParsedSigningRequest {
	stakerPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	finalityProviderKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	stakingTime := uint16(10000)
	unbondingTime := uint16(1000)
	stakingAmount := btcutil.Amount(100000)
	unbondingFee := btcutil.Amount(1000)
	slashingFee := btcutil.Amount(1000)
	slashingRate := sdkmath.LegacyMustNewDecFromStr("0.1")

	fakeInput := wire.NewTxIn(wire.NewOutPoint(&chainhash.Hash{}, 0), nil, nil)
	stakingInfo, err := btcstaking.BuildStakingInfo(
		stakerPrivKey.PubKey(),
		[]*btcec.PublicKey{finalityProviderKey.PubKey()},
		[]*btcec.PublicKey{covnenantPublicKey},
		1,
		stakingTime,
		stakingAmount,
		netParams,
	)
	require.NoError(t, err)

	stakingTx := wire.NewMsgTx(2)
	stakingTx.AddTxIn(fakeInput)
	stakingTx.AddTxOut(stakingInfo.StakingOutput)

	stakingSlashingSpendInfo, err := stakingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)
	stakingUnbondingSpendInfo, err := stakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)

	stakingSlashingScript := stakingSlashingSpendInfo.RevealedLeaf.Script
	stakingUnbondingScript := stakingUnbondingSpendInfo.RevealedLeaf.Script

	unbondingInfo, err := staking.BuildUnbondingInfo(
		stakerPrivKey.PubKey(),
		[]*btcec.PublicKey{finalityProviderKey.PubKey()},
		[]*btcec.PublicKey{covnenantPublicKey},
		1,
		unbondingTime,
		stakingAmount-unbondingFee,
		netParams,
	)
	require.NoError(t, err)

	unbondingSlashingSpendInfo, err := unbondingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)
	unbondingSlashingScript := unbondingSlashingSpendInfo.RevealedLeaf.Script

	stakingTxHash := stakingTx.TxHash()
	stakingOutputIndex := uint32(0)

	unbondingTx := wire.NewMsgTx(2)
	unbondingTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&stakingTxHash, stakingOutputIndex), nil, nil))
	unbondingTx.AddTxOut(unbondingInfo.UnbondingOutput)

	stakingSlashingTx, err := btcstaking.BuildSlashingTxFromStakingTxStrict(
		stakingTx,
		stakingOutputIndex,
		stakingSlashingScript,
		stakerPrivKey.PubKey(),
		unbondingTime,
		int64(slashingFee),
		slashingRate,
		netParams,
	)
	require.NoError(t, err)

	unbondingSlashingTx, err := btcstaking.BuildSlashingTxFromStakingTxStrict(
		unbondingTx,
		0,
		unbondingSlashingScript,
		stakerPrivKey.PubKey(),
		unbondingTime,
		int64(slashingFee),
		slashingRate,
		netParams,
	)
	require.NoError(t, err)

	fpEncKey, err := asig.NewEncryptionKeyFromBTCPK(finalityProviderKey.PubKey())
	require.NoError(t, err)

	return signerapp.ParsedSigningRequest{
		StakingTx:               stakingTx,
		SlashingTx:              stakingSlashingTx,
		UnbondingTx:             unbondingTx,
		SlashUnbondingTx:        unbondingSlashingTx,
		StakingOutputIdx:        stakingOutputIndex,
		SlashingScript:          stakingSlashingScript,
		UnbondingScript:         stakingUnbondingScript,
		UnbondingSlashingScript: unbondingSlashingScript,
		FpEncKeys:               []*asig.EncryptionKey{fpEncKey},
	}
}

func TestSigningTransactions(t *testing.T) {
	tm := StartManager(t, 100)

	dataToSign := buildDataToSign(t, tm.covenantPrivKey.PubKey())

	sigs, err := signerservice.RequestCovenantSignaure(
		context.Background(),
		tm.SigningServerUrl(),
		10*time.Second,
		&dataToSign,
	)

	require.NoError(t, err)
	require.NotNil(t, sigs)

	err = tm.verifyResponse(sigs, &dataToSign)
	require.NoError(t, err)
}

func (tm *TestManager) verifyResponse(resp *signerapp.ParsedSigningResponse, req *signerapp.ParsedSigningRequest) error {

	slashAdaptorSig, err := asig.NewAdaptorSignatureFromBytes(resp.SlashAdaptorSigs[0])

	if err != nil {
		return err
	}

	err = btcstaking.EncVerifyTransactionSigWithOutput(
		req.SlashingTx,
		req.StakingTx.TxOut[req.StakingOutputIdx],
		req.SlashingScript,
		tm.covenantPrivKey.PubKey(),
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
		tm.covenantPrivKey.PubKey(),
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
		tm.covenantPrivKey.PubKey(),
		resp.UnbondingSig.Serialize(),
	)

	if err != nil {
		return fmt.Errorf("failed to verify unbonding signature for unbonding tx: %w", err)
	}

	return nil
}

func TestRejectToLargeRequest(t *testing.T) {
	tm := StartManager(t, 100)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	tmContentLimit := tm.signerConfig.Server.MaxContentLength
	size := tmContentLimit + 1
	tooLargeTx := datagen.GenRandomByteArray(r, uint64(size))

	req := types.SignTransactionsRequest{
		StakingTxHex:   "",
		UnbondingTxHex: hex.EncodeToString(tooLargeTx),
	}

	marshalled, err := json.Marshal(req)
	require.NoError(t, err)

	route := fmt.Sprintf("%s/v1/sign-transactions", tm.SigningServerUrl())

	httpRequest, err := http.NewRequestWithContext(context.Background(), "POST", route, bytes.NewReader(marshalled))
	require.NoError(t, err)

	// use json
	httpRequest.Header.Set("Content-Type", "application/json")

	client := http.Client{Timeout: 10 * time.Second}
	// send the request
	res, err := client.Do(httpRequest)
	require.NoError(t, err)
	require.NotNil(t, res)
	defer res.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, res.StatusCode)
}
