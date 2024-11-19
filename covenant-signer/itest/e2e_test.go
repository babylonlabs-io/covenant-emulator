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

	"github.com/babylonlabs-io/babylon/btcstaking"
	staking "github.com/babylonlabs-io/babylon/btcstaking"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/babylonlabs-io/networks/parameters/parser"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/btcclient"
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
	t                     *testing.T
	bitcoindHandler       *BitcoindTestHandler
	walletPass            string
	btcClient             *btcclient.BtcClient
	localCovenantPubKey   *btcec.PublicKey
	allCovenantKeys       []*btcec.PublicKey
	covenantQuorum        uint32
	finalityProviderKey   *btcec.PrivateKey
	walletAddress         btcutil.Address
	stakerPrivKey         *btcec.PrivateKey
	stakerPubKey          *btcec.PublicKey
	magicBytes            []byte
	requiredUnbondingTime uint16
	confirmationDepth     uint16
	requiredUnbondingFee  btcutil.Amount
	signerConfig          *config.Config
	app                   *signerapp.SignerApp
	server                *signerservice.SigningServer
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
	appConfig.BtcNodeConfig.Host = "127.0.0.1:18443"
	appConfig.BtcNodeConfig.User = "user"
	appConfig.BtcNodeConfig.Pass = "pass"
	appConfig.BtcNodeConfig.Network = netParams.Name

	fakeParsedConfig, err := appConfig.Parse()
	require.NoError(t, err)
	// Client for testing purposes
	client, err := btcclient.NewBtcClient(fakeParsedConfig.BtcNodeConfig)
	require.NoError(t, err)

	outputs, err := client.ListOutputs(true)
	require.NoError(t, err)
	require.Len(t, outputs, int(numMatureOutputsInWallet))

	// easiest way to get address controlled by wallet is to retrive address from one
	// of the outputs
	output := outputs[0]
	walletAddress, err := btcutil.DecodeAddress(output.Address, netParams)
	require.NoError(t, err)

	// Unlock wallet for all tests 60min
	err = client.UnlockWallet(60*60*60, passphrase)
	require.NoError(t, err)

	stakerPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	stakerPubKey := stakerPrivKey.PubKey()

	fpKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	covAddress, err := client.RpcClient.GetNewAddress("covenant")
	require.NoError(t, err)
	info, err := client.RpcClient.GetAddressInfo(covAddress.EncodeAddress())
	require.NoError(t, err)
	covenantPubKeyBytes, err := hex.DecodeString(*info.PubKey)
	require.NoError(t, err)
	localCovenantKey, err := btcec.ParsePubKey(covenantPubKeyBytes)
	require.NoError(t, err)

	remoteCovenantKey1, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	require.NotNil(t, remoteCovenantKey1)
	remoteCovenantKey2, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	require.NotNil(t, remoteCovenantKey2)

	mb := []byte{0x0, 0x1, 0x2, 0x3}
	appConfig.Server.Host = "127.0.0.1"
	appConfig.Server.Port = 10090

	testParams := parser.VersionedGlobalParams{}
	testParams.ActivationHeight = 1
	testParams.StakingCap = 10000000000
	testParams.Tag = hex.EncodeToString(mb)
	testParams.CovenantPks = []string{
		hex.EncodeToString(localCovenantKey.SerializeCompressed()),
		hex.EncodeToString(remoteCovenantKey1.PubKey().SerializeCompressed()),
		hex.EncodeToString(remoteCovenantKey2.PubKey().SerializeCompressed()),
	}
	testParams.CovenantQuorum = 2
	testParams.UnbondingTime = 100
	testParams.UnbondingFee = 1000
	testParams.MinStakingTime = 10000
	testParams.MaxStakingTime = 10000
	testParams.MinStakingAmount = 10000
	testParams.MaxStakingAmount = 10000000
	testParams.ConfirmationDepth = 10

	// TODO: Update tests to create json file and read from it.
	globalParams := parser.GlobalParams{
		Versions: []*parser.VersionedGlobalParams{
			&testParams,
		},
	}

	parsedGlobalParams, err := parser.ParseGlobalParams(&globalParams)
	require.NoError(t, err)

	parsedconfig, err := appConfig.Parse()
	require.NoError(t, err)

	// In e2e test we are using the same node for signing as for indexing functionalities
	chainInfo := signerapp.NewBitcoindChainInfo(client)
	signer := signerapp.NewPsbtSigner(client)

	app := signerapp.NewSignerApp(
		signer,
		chainInfo,
		&signerapp.VersionedParamsRetriever{parsedGlobalParams},
		netParams,
	)

	met := metrics.NewCovenantSignerMetrics()

	server, err := signerservice.New(
		context.Background(),
		parsedconfig,
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
		t:                     t,
		bitcoindHandler:       h,
		walletPass:            passphrase,
		btcClient:             client,
		localCovenantPubKey:   localCovenantKey,
		allCovenantKeys:       parsedGlobalParams.Versions[0].CovenantPks,
		covenantQuorum:        parsedGlobalParams.Versions[0].CovenantQuorum,
		requiredUnbondingTime: parsedGlobalParams.Versions[0].UnbondingTime,
		requiredUnbondingFee:  parsedGlobalParams.Versions[0].UnbondingFee,
		confirmationDepth:     parsedGlobalParams.Versions[0].ConfirmationDepth,
		finalityProviderKey:   fpKey,
		walletAddress:         walletAddress,
		stakerPrivKey:         stakerPrivKey,
		stakerPubKey:          stakerPubKey,
		magicBytes:            mb,
		signerConfig:          appConfig,
		app:                   app,
		server:                server,
	}
}

func (tm *TestManager) covenantPubKeys() []*btcec.PublicKey {
	return tm.allCovenantKeys
}

func (tm *TestManager) SigningServerUrl() string {
	return fmt.Sprintf("http://%s:%d", tm.signerConfig.Server.Host, tm.signerConfig.Server.Port)
}

type stakingTxSigInfo struct {
	stakingTxHash *chainhash.Hash
	stakingOutput *wire.TxOut
	stakingInfo   *btcstaking.IdentifiableStakingInfo
}

func (tm *TestManager) sendStakingTxToBtc(d *stakingData) *stakingTxSigInfo {
	info, err := staking.BuildV0IdentifiableStakingOutputs(
		tm.magicBytes,
		tm.stakerPubKey,
		tm.finalityProviderKey.PubKey(),
		tm.covenantPubKeys(),
		tm.covenantQuorum,
		d.stakingTime,
		d.stakingAmount,
		netParams,
	)
	require.NoError(tm.t, err)

	// staking output will always have index 0
	tx, err := tm.btcClient.CreateAndSignTx(
		[]*wire.TxOut{info.StakingOutput, info.OpReturnOutput},
		d.stakingFeeRate,
		tm.walletAddress,
	)
	require.NoError(tm.t, err)

	hash, err := tm.btcClient.SendTx(tx)
	require.NoError(tm.t, err)
	// generate exact amount of block to confirm staking tx
	_ = tm.bitcoindHandler.GenerateBlocks(int(tm.confirmationDepth))
	return &stakingTxSigInfo{
		stakingTxHash: hash,
		stakingOutput: info.StakingOutput,
		stakingInfo:   info,
	}
}

type unbondingTxWithMetadata struct {
	unbondingTx *wire.MsgTx
}

func (tm *TestManager) createUnbondingTx(
	si *stakingTxSigInfo,
	d *stakingData,
) *unbondingTxWithMetadata {

	unbondingInfo, err := staking.BuildUnbondingInfo(
		tm.stakerPubKey,
		[]*btcec.PublicKey{tm.finalityProviderKey.PubKey()},
		tm.covenantPubKeys(),
		tm.covenantQuorum,
		tm.requiredUnbondingTime,
		d.stakingAmount-tm.requiredUnbondingFee,
		netParams,
	)
	require.NoError(tm.t, err)
	unbondingTx := wire.NewMsgTx(2)
	unbondingTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(si.stakingTxHash, 0), nil, nil))
	unbondingTx.AddTxOut(unbondingInfo.UnbondingOutput)

	return &unbondingTxWithMetadata{
		unbondingTx: unbondingTx,
	}
}

func (tm *TestManager) createNUnbondingTransactions(n int, d *stakingData) ([]*unbondingTxWithMetadata, []*wire.MsgTx) {
	var infos []*stakingTxSigInfo
	var sendStakingTransactions []*wire.MsgTx

	for i := 0; i < n; i++ {
		sInfo := tm.sendStakingTxToBtc(d)
		conf, status, err := tm.btcClient.TxDetails(sInfo.stakingTxHash, sInfo.stakingOutput.PkScript)
		require.NoError(tm.t, err)
		require.Equal(tm.t, btcclient.TxInChain, status)
		infos = append(infos, sInfo)
		sendStakingTransactions = append(sendStakingTransactions, conf.Tx)
	}

	var unbondingTxs []*unbondingTxWithMetadata
	for _, i := range infos {
		info := i
		ubs := tm.createUnbondingTx(
			info,
			d,
		)
		unbondingTxs = append(unbondingTxs, ubs)
	}

	return unbondingTxs, sendStakingTransactions
}

func TestSigningUnbondingTx(t *testing.T) {
	tm := StartManager(t, 100)

	stakingData := defaultStakingData()

	stakingTxInfo := tm.sendStakingTxToBtc(stakingData)

	unb := tm.createUnbondingTx(stakingTxInfo, stakingData)

	// staker signs unbonding tx
	unbondingPathInfo, err := stakingTxInfo.stakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)

	stakerSig, err := btcstaking.SignTxWithOneScriptSpendInputFromTapLeaf(
		unb.unbondingTx,
		stakingTxInfo.stakingOutput,
		tm.stakerPrivKey,
		unbondingPathInfo.RevealedLeaf,
	)
	require.NoError(t, err)

	sig, err := signerservice.RequestCovenantSignaure(
		context.Background(),
		tm.SigningServerUrl(),
		10*time.Second,
		unb.unbondingTx,
		stakerSig,
		tm.localCovenantPubKey,
		stakingTxInfo.stakingOutput.PkScript,
	)

	require.NoError(t, err)
	require.NotNil(t, sig)

	// check if signature provided by covenant signer is valid signature over unbonding
	// path
	err = btcstaking.VerifyTransactionSigWithOutput(
		unb.unbondingTx,
		stakingTxInfo.stakingOutput,
		unbondingPathInfo.GetPkScriptPath(),
		tm.localCovenantPubKey,
		sig.Serialize(),
	)
	require.NoError(t, err)
}

func TestProperResponseForInvalidRequest(t *testing.T) {
	tm := StartManager(t, 100)

	stakingData := defaultStakingData()

	stakingTxInfo := tm.sendStakingTxToBtc(stakingData)

	unb := tm.createUnbondingTx(stakingTxInfo, stakingData)

	// staker signs unbonding tx
	unbondingPathInfo, err := stakingTxInfo.stakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)

	randomKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	// We will send invalid signature in request, server should respond with
	// bad request
	badSig, err := btcstaking.SignTxWithOneScriptSpendInputFromTapLeaf(
		unb.unbondingTx,
		stakingTxInfo.stakingOutput,
		randomKey,
		unbondingPathInfo.RevealedLeaf,
	)
	require.NoError(t, err)

	sig, err := signerservice.RequestCovenantSignaure(
		context.Background(),
		tm.SigningServerUrl(),
		10*time.Second,
		unb.unbondingTx,
		badSig,
		tm.localCovenantPubKey,
		stakingTxInfo.stakingOutput.PkScript,
	)

	require.Error(t, err)
	require.Nil(t, sig)
	require.EqualError(t, err, "signing request failed. status code: 400, message: {\"errorCode\":\"BAD_REQUEST\",\"message\":\"staker unbonding signature verification failed: signature is not valid: invalid signing request\"}")
}

func TestRejectToLargeRequest(t *testing.T) {
	tm := StartManager(t, 100)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	tmContentLimit := tm.signerConfig.Server.MaxContentLength
	size := tmContentLimit + 1
	tooLargeTx := datagen.GenRandomByteArray(r, uint64(size))

	req := types.SignUnbondingTxRequest{
		StakingOutputPkScriptHex: "",
		UnbondingTxHex:           hex.EncodeToString(tooLargeTx),
		StakerUnbondingSigHex:    "",
		CovenantPublicKey:        "",
	}

	marshalled, err := json.Marshal(req)
	require.NoError(t, err)

	route := fmt.Sprintf("%s/v1/sign-unbonding-tx", tm.SigningServerUrl())

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
