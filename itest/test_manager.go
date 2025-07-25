package e2etest

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	appparams "github.com/babylonlabs-io/babylon/v3/app/params"
	"github.com/babylonlabs-io/babylon/v3/app/signingcontext"
	"github.com/babylonlabs-io/babylon/v3/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	btcctypes "github.com/babylonlabs-io/babylon/v3/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/v3/x/btclightclient/types"
	bstypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	covcc "github.com/babylonlabs-io/covenant-emulator/clientcontroller"
	covcfg "github.com/babylonlabs-io/covenant-emulator/config"
	"github.com/babylonlabs-io/covenant-emulator/covenant"
	signerCfg "github.com/babylonlabs-io/covenant-emulator/covenant-signer/config"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/keystore/cosmos"
	signerMetrics "github.com/babylonlabs-io/covenant-emulator/covenant-signer/observability/metrics"
	signerApp "github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice"
	signerService "github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice"
	"github.com/babylonlabs-io/covenant-emulator/remotesigner"
	"github.com/babylonlabs-io/covenant-emulator/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var (
	eventuallyWaitTimeOut = 1 * time.Minute
	eventuallyPollTime    = 500 * time.Millisecond
	btcNetworkParams      = &chaincfg.SimNetParams

	chainID    = "chain-test"
	passphrase = "testpass"
	hdPath     = ""
)

type TestManager struct {
	Wg               sync.WaitGroup
	BabylonHandler   *BabylonNodeHandler
	CovenantEmulator *covenant.CovenantEmulator
	CovenanConfig    *covcfg.Config
	CovBBNClient     *covcc.BabylonController
	StakingParams    *types.StakingParams
	baseDir          string
}

type TestDelegationData struct {
	DelegatorPrivKey *btcec.PrivateKey
	DelegatorKey     *btcec.PublicKey
	SlashingTx       *bstypes.BTCSlashingTx
	StakingTx        *wire.MsgTx
	StakingTxInfo    *btcctypes.TransactionInfo
	DelegatorSig     *bbntypes.BIP340Signature
	FpPks            []*btcec.PublicKey

	SlashingPkScript []byte
	StakingTime      uint16
	StakingAmount    int64
}

type testFinalityProviderData struct {
	BabylonAddress sdk.AccAddress
	BtcPrivKey     *btcec.PrivateKey
	BtcKey         *btcec.PublicKey
	PoP            *bstypes.ProofOfPossessionBTC
}

func StartManager(t *testing.T, hmacKey string) *TestManager {
	testDir, err := baseDir("cee2etest")
	require.NoError(t, err)

	logger := zap.NewNop()
	covenantConfig := defaultCovenantConfig(testDir)
	err = covenantConfig.Validate()
	require.NoError(t, err)

	// 1. prepare covenant key, which will be used as input of Babylon node
	signerConfig := signerCfg.DefaultConfig()
	signerConfig.KeyStore.CosmosKeyStore.ChainID = covenantConfig.BabylonConfig.ChainID
	signerConfig.KeyStore.CosmosKeyStore.KeyName = covenantConfig.BabylonConfig.Key
	signerConfig.KeyStore.CosmosKeyStore.KeyringBackend = covenantConfig.BabylonConfig.KeyringBackend
	signerConfig.KeyStore.CosmosKeyStore.KeyDirectory = covenantConfig.BabylonConfig.KeyDirectory
	keyRetriever, err := cosmos.NewCosmosKeyringRetriever(signerConfig.KeyStore.CosmosKeyStore)
	require.NoError(t, err)
	keyInfo, err := keyRetriever.Kr.CreateChainKey(
		passphrase,
		hdPath,
	)
	require.NoError(t, err)
	require.NotNil(t, keyInfo)

	app := signerApp.NewSignerApp(
		keyRetriever,
	)

	met := signerMetrics.NewCovenantSignerMetrics()
	parsedConfig, err := signerConfig.Parse()
	require.NoError(t, err)

	remoteSignerPort, url := AllocateUniquePort(t)
	parsedConfig.ServerConfig.Port = remoteSignerPort

	// Configure HMAC keys if provided
	if hmacKey != "" {
		parsedConfig.ServerConfig.HMACKey = hmacKey
		covenantConfig.RemoteSigner.HMACKey = hmacKey
	}

	covenantConfig.RemoteSigner.URL = fmt.Sprintf("http://%s", url)

	server, err := signerService.New(
		context.Background(),
		parsedConfig,
		app,
		met,
	)
	require.NoError(t, err)

	signer := remotesigner.NewRemoteSigner(covenantConfig.RemoteSigner)
	covPubKey := keyInfo.PublicKey

	go func() {
		_ = server.Start()
	}()

	// Give some time to launch server
	time.Sleep(3 * time.Second)

	// unlock the signer before usage
	err = signerservice.Unlock(
		context.Background(),
		covenantConfig.RemoteSigner.URL,
		covenantConfig.RemoteSigner.Timeout,
		passphrase,
		covenantConfig.RemoteSigner.HMACKey,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = server.Stop(context.TODO())
	})

	// 2. prepare Babylon node
	bh := NewBabylonNodeHandler(t, bbntypes.NewBIP340PubKeyFromBTCPK(covPubKey))
	err = bh.Start()
	require.NoError(t, err)

	// 3. prepare covenant emulator
	bbnCfg := defaultBBNConfigWithKey("test-spending-key", bh.GetNodeDataDir())
	covbc, err := covcc.NewBabylonController(bbnCfg, &covenantConfig.BTCNetParams, logger)
	require.NoError(t, err)

	require.NoError(t, err)

	ce, err := covenant.NewCovenantEmulator(covenantConfig, covbc, logger, signer)
	require.NoError(t, err)
	err = ce.Start()
	require.NoError(t, err)

	tm := &TestManager{
		BabylonHandler:   bh,
		CovenantEmulator: ce,
		CovenanConfig:    covenantConfig,
		CovBBNClient:     covbc,
		baseDir:          testDir,
	}

	tm.WaitForServicesStart(t)
	tm.SendToAddr(t, keyInfo.Address.String(), "100000ubbn")

	return tm
}

func (tm *TestManager) WaitForServicesStart(t *testing.T) {
	// wait for Babylon node starts
	require.Eventually(t, func() bool {
		params, err := tm.CovBBNClient.QueryStakingParamsByVersion(0)
		if err != nil {
			return false
		}
		tm.StakingParams = params
		return true
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("Babylon node is started")
}

func (tm *TestManager) SendToAddr(t *testing.T, toAddr, amount string) {
	sendTx := exec.Command(
		"babylond",
		"tx",
		"bank",
		"send",
		"node0",
		toAddr,
		amount,
		"--keyring-backend=test",
		"--chain-id=chain-test",
		fmt.Sprintf("--home=%s", tm.BabylonHandler.babylonNode.nodeHome),
	)
	err := sendTx.Start()
	require.NoError(t, err)
}

func StartManagerWithFinalityProvider(t *testing.T, n int) (*TestManager, []*btcec.PublicKey) {
	tm := StartManager(t, "")
	// fund the finality provider operator account
	// to submit the registration tx
	tm.SendToAddr(t, tm.CovBBNClient.GetKeyAddress().String(), "100000ubbn")

	var btcPks []*btcec.PublicKey
	for i := 0; i < n; i++ {
		fpData := genTestFinalityProviderData(
			t,
			tm.CovenanConfig.BabylonConfig.ChainID,
			tm.CovBBNClient.GetKeyAddress(),
		)
		btcPubKey := bbntypes.NewBIP340PubKeyFromBTCPK(fpData.BtcKey)
		_, err := tm.CovBBNClient.RegisterFinalityProvider(
			btcPubKey,
			&tm.StakingParams.MinComissionRate,
			&stakingtypes.Description{
				Moniker: "tester",
			},
			fpData.PoP,
		)
		require.NoError(t, err)

		btcPks = append(btcPks, fpData.BtcKey)
	}

	// check finality providers on Babylon side
	require.Eventually(t, func() bool {
		fps, err := tm.CovBBNClient.QueryFinalityProviders()
		if err != nil {
			t.Logf("failed to query finality providers from Babylon %s", err.Error())
			return false
		}

		return len(fps) == n
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("the test manager is running with %v finality-provider(s)", n)

	return tm, btcPks
}

func genTestFinalityProviderData(t *testing.T, chainID string, babylonAddr sdk.AccAddress) *testFinalityProviderData {
	fpPopContext := signingcontext.FpPopContextV0(chainID, appparams.AccBTCStaking.String())
	finalityProviderEOTSPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	pop, err := datagen.NewPoPBTC(fpPopContext, babylonAddr, finalityProviderEOTSPrivKey)
	require.NoError(t, err)

	return &testFinalityProviderData{
		BabylonAddress: babylonAddr,
		BtcPrivKey:     finalityProviderEOTSPrivKey,
		BtcKey:         finalityProviderEOTSPrivKey.PubKey(),
		PoP:            pop,
	}
}

func (tm *TestManager) Stop(t *testing.T) {
	err := tm.CovenantEmulator.Stop()
	require.NoError(t, err)
	err = tm.BabylonHandler.Stop()
	require.NoError(t, err)
	err = os.RemoveAll(tm.baseDir)
	require.NoError(t, err)
}

func (tm *TestManager) WaitForNPendingDels(t *testing.T, n int) []*types.Delegation {
	var (
		dels []*types.Delegation
		err  error
	)
	require.Eventually(t, func() bool {
		dels, err = tm.CovBBNClient.QueryPendingDelegations(
			tm.CovenanConfig.DelegationLimit,
			nil,
		)
		if err != nil {
			return false
		}
		return len(dels) == n
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("delegations are pending")

	return dels
}

// waitForNDelsWithStatus is a generic method for waiting for delegations with a specific status
func (tm *TestManager) waitForNDelsWithStatus(t *testing.T, n int, queryFunc func(uint64) ([]*types.Delegation, error), statusName string) []*types.Delegation {
	var (
		dels []*types.Delegation
		err  error
	)
	require.Eventually(t, func() bool {
		dels, err = queryFunc(tm.CovenanConfig.DelegationLimit)
		if err != nil {
			return false
		}
		return len(dels) == n
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("delegations are %s", statusName)

	return dels
}

func (tm *TestManager) WaitForNActiveDels(t *testing.T, n int) []*types.Delegation {
	return tm.waitForNDelsWithStatus(t, n, tm.CovBBNClient.QueryActiveDelegations, "active")
}

func (tm *TestManager) WaitForNVerifiedDels(t *testing.T, n int) []*types.Delegation {
	return tm.waitForNDelsWithStatus(t, n, tm.CovBBNClient.QueryVerifiedDelegations, "verified")
}

// InsertBTCDelegation inserts a BTC delegation to Babylon
// isPreApproval indicates whether the delegation follows
// pre-approval flow, if so, the inclusion proof is nil
func (tm *TestManager) InsertBTCDelegation(
	t *testing.T,
	fpPks []*btcec.PublicKey, stakingTime uint16, stakingAmount int64,
	isPreApproval bool,
) *TestDelegationData {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	params := tm.StakingParams

	// delegator BTC key pairs, staking tx and slashing tx
	delBtcPrivKey, delBtcPubKey, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)

	unbondingTime := uint16(tm.StakingParams.UnbondingTimeBlocks)
	testStakingInfo := datagen.GenBTCStakingSlashingInfo(
		r,
		t,
		btcNetworkParams,
		delBtcPrivKey,
		fpPks,
		params.CovenantPks,
		params.CovenantQuorum,
		stakingTime,
		stakingAmount,
		params.SlashingPkScript,
		params.SlashingRate,
		unbondingTime,
	)

	// proof-of-possession
	stakerPopContext := signingcontext.StakerPopContextV0(tm.CovenanConfig.BabylonConfig.ChainID, appparams.AccBTCStaking.String())
	pop, err := datagen.NewPoPBTC(stakerPopContext, tm.CovBBNClient.GetKeyAddress(), delBtcPrivKey)
	require.NoError(t, err)

	// create and insert BTC headers which include the staking tx to get staking tx info
	currentBtcTipResp, err := tm.CovBBNClient.QueryBtcLightClientTip()
	require.NoError(t, err)
	tipHeader, err := bbntypes.NewBTCHeaderBytesFromHex(currentBtcTipResp.HeaderHex)
	require.NoError(t, err)
	blockWithStakingTx := datagen.CreateBlockWithTransaction(r, tipHeader.ToBlockHeader(), testStakingInfo.StakingTx)
	accumulatedWork := btclctypes.CalcWork(&blockWithStakingTx.HeaderBytes)
	accumulatedWork = btclctypes.CumulativeWork(accumulatedWork, currentBtcTipResp.Work)
	parentBlockHeaderInfo := &btclctypes.BTCHeaderInfo{
		Header: &blockWithStakingTx.HeaderBytes,
		Hash:   blockWithStakingTx.HeaderBytes.Hash(),
		Height: currentBtcTipResp.Height + 1,
		Work:   &accumulatedWork,
	}
	headers := make([]bbntypes.BTCHeaderBytes, 0)
	headers = append(headers, blockWithStakingTx.HeaderBytes)
	for i := 0; i < int(params.ComfirmationTimeBlocks); i++ {
		headerInfo := datagen.GenRandomValidBTCHeaderInfoWithParent(r, *parentBlockHeaderInfo)
		headers = append(headers, *headerInfo.Header)
		parentBlockHeaderInfo = headerInfo
	}
	_, err = tm.CovBBNClient.InsertBtcBlockHeaders(headers)
	require.NoError(t, err)
	btcHeader := blockWithStakingTx.HeaderBytes
	serializedStakingTx, err := bbntypes.SerializeBTCTx(testStakingInfo.StakingTx)
	require.NoError(t, err)
	txInfo := btcctypes.NewTransactionInfo(&btcctypes.TransactionKey{Index: 1, Hash: btcHeader.Hash()}, serializedStakingTx, blockWithStakingTx.SpvProof.MerkleNodes)

	slashingSpendInfo, err := testStakingInfo.StakingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)

	// delegator sig
	delegatorSig, err := testStakingInfo.SlashingTx.Sign(
		testStakingInfo.StakingTx,
		datagen.StakingOutIdx,
		slashingSpendInfo.GetPkScriptPath(),
		delBtcPrivKey,
	)
	require.NoError(t, err)

	unbondingValue := stakingAmount - 1000
	stakingTxHash := testStakingInfo.StakingTx.TxHash()

	testUnbondingInfo := datagen.GenBTCUnbondingSlashingInfo(
		r,
		t,
		btcNetworkParams,
		delBtcPrivKey,
		fpPks,
		params.CovenantPks,
		params.CovenantQuorum,
		wire.NewOutPoint(&stakingTxHash, datagen.StakingOutIdx),
		unbondingTime,
		unbondingValue,
		params.SlashingPkScript,
		params.SlashingRate,
		unbondingTime,
	)

	unbondingTxMsg := testUnbondingInfo.UnbondingTx

	unbondingSlashingPathInfo, err := testUnbondingInfo.UnbondingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)

	unbondingSig, err := testUnbondingInfo.SlashingTx.Sign(
		unbondingTxMsg,
		datagen.StakingOutIdx,
		unbondingSlashingPathInfo.GetPkScriptPath(),
		delBtcPrivKey,
	)
	require.NoError(t, err)

	serializedUnbondingTx, err := bbntypes.SerializeBTCTx(testUnbondingInfo.UnbondingTx)
	require.NoError(t, err)

	// submit the BTC delegation to Babylon
	_, err = tm.CovBBNClient.CreateBTCDelegation(
		bbntypes.NewBIP340PubKeyFromBTCPK(delBtcPubKey),
		fpPks,
		pop,
		uint32(stakingTime),
		stakingAmount,
		txInfo,
		testStakingInfo.SlashingTx,
		delegatorSig,
		serializedUnbondingTx,
		uint32(unbondingTime),
		unbondingValue,
		testUnbondingInfo.SlashingTx,
		unbondingSig,
		isPreApproval)
	require.NoError(t, err)

	t.Log("successfully submitted a BTC delegation")

	return &TestDelegationData{
		DelegatorPrivKey: delBtcPrivKey,
		DelegatorKey:     delBtcPubKey,
		FpPks:            fpPks,
		StakingTx:        testStakingInfo.StakingTx,
		SlashingTx:       testStakingInfo.SlashingTx,
		StakingTxInfo:    txInfo,
		DelegatorSig:     delegatorSig,
		SlashingPkScript: params.SlashingPkScript,
		StakingTime:      stakingTime,
		StakingAmount:    stakingAmount,
	}
}

// InsertStakeExpansionDelegation inserts a BTC stake expansion delegation to Babylon
func (tm *TestManager) InsertStakeExpansionDelegation(
	t *testing.T,
	fpPks []*btcec.PublicKey,
	stakingTime uint16,
	stakingAmount int64,
	prevStakingTx *wire.MsgTx,
	isPreApproval bool,
) *TestDelegationData {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	params := tm.StakingParams

	// delegator BTC key pairs, staking tx and slashing tx
	delBtcPrivKey, delBtcPubKey, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)

	unbondingTime := uint16(tm.StakingParams.UnbondingTimeBlocks)

	// Convert previousStakingTxHash string to OutPoint
	previousStakingTxHash := prevStakingTx.TxHash().String()
	prevHash, err := chainhash.NewHashFromStr(previousStakingTxHash)
	require.NoError(t, err)
	prevStakingOutPoint := wire.NewOutPoint(prevHash, datagen.StakingOutIdx)

	// Generate a simple funding transaction for the stake expansion
	stakingOutput := prevStakingTx.TxOut[0]

	// Create a fake outPoint for funding
	dummyData := sha256.Sum256([]byte("dummy funding tx"))
	dummyOutPoint := &wire.OutPoint{
		Hash:  chainhash.Hash(dummyData),
		Index: 0,
	}

	// Generate funding tx for stake expansion
	fundingTx := datagen.GenFundingTx(
		t,
		r,
		btcNetworkParams,
		dummyOutPoint,
		stakingAmount,
		stakingOutput,
	)

	// Convert fundingTxHash to OutPoint
	fundingTxHash := fundingTx.TxHash()
	fundingOutPoint := wire.NewOutPoint(&fundingTxHash, 0)
	outPoints := []*wire.OutPoint{prevStakingOutPoint, fundingOutPoint}

	// Generate staking slashing info using the previous staking outpoint
	// For stake expansion, we create a transaction with multiple inputs
	testStakingInfo := datagen.GenBTCStakingSlashingInfoWithInputs(
		r,
		t,
		btcNetworkParams,
		outPoints,
		delBtcPrivKey,
		fpPks,
		params.CovenantPks,
		params.CovenantQuorum,
		stakingTime,
		stakingAmount,
		params.SlashingPkScript,
		params.SlashingRate,
		unbondingTime,
	)

	// proof-of-possession
	stakerPopContext := signingcontext.StakerPopContextV0(tm.CovenanConfig.BabylonConfig.ChainID, appparams.AccBTCStaking.String())
	pop, err := datagen.NewPoPBTC(stakerPopContext, tm.CovBBNClient.GetKeyAddress(), delBtcPrivKey)
	require.NoError(t, err)

	// create and insert BTC headers which include the staking tx to get staking tx info
	currentBtcTipResp, err := tm.CovBBNClient.QueryBtcLightClientTip()
	require.NoError(t, err)
	tipHeader, err := bbntypes.NewBTCHeaderBytesFromHex(currentBtcTipResp.HeaderHex)
	require.NoError(t, err)
	blockWithStakingTx := datagen.CreateBlockWithTransaction(r, tipHeader.ToBlockHeader(), testStakingInfo.StakingTx)
	accumulatedWork := btclctypes.CalcWork(&blockWithStakingTx.HeaderBytes)
	accumulatedWork = btclctypes.CumulativeWork(accumulatedWork, currentBtcTipResp.Work)
	parentBlockHeaderInfo := &btclctypes.BTCHeaderInfo{
		Header: &blockWithStakingTx.HeaderBytes,
		Hash:   blockWithStakingTx.HeaderBytes.Hash(),
		Height: currentBtcTipResp.Height + 1,
		Work:   &accumulatedWork,
	}
	headers := make([]bbntypes.BTCHeaderBytes, 0)
	headers = append(headers, blockWithStakingTx.HeaderBytes)
	for i := 0; i < int(params.ComfirmationTimeBlocks); i++ {
		headerInfo := datagen.GenRandomValidBTCHeaderInfoWithParent(r, *parentBlockHeaderInfo)
		headers = append(headers, *headerInfo.Header)
		parentBlockHeaderInfo = headerInfo
	}
	_, err = tm.CovBBNClient.InsertBtcBlockHeaders(headers)
	require.NoError(t, err)
	btcHeader := blockWithStakingTx.HeaderBytes
	serializedStakingTx, err := bbntypes.SerializeBTCTx(testStakingInfo.StakingTx)
	require.NoError(t, err)
	txInfo := btcctypes.NewTransactionInfo(&btcctypes.TransactionKey{Index: 1, Hash: btcHeader.Hash()}, serializedStakingTx, blockWithStakingTx.SpvProof.MerkleNodes)

	slashingSpendInfo, err := testStakingInfo.StakingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)

	// delegator sig
	delegatorSig, err := testStakingInfo.SlashingTx.Sign(
		testStakingInfo.StakingTx,
		datagen.StakingOutIdx,
		slashingSpendInfo.GetPkScriptPath(),
		delBtcPrivKey,
	)
	require.NoError(t, err)

	unbondingValue := stakingAmount - 1000
	stakingTxHash := testStakingInfo.StakingTx.TxHash()

	testUnbondingInfo := datagen.GenBTCUnbondingSlashingInfo(
		r,
		t,
		btcNetworkParams,
		delBtcPrivKey,
		fpPks,
		params.CovenantPks,
		params.CovenantQuorum,
		wire.NewOutPoint(&stakingTxHash, datagen.StakingOutIdx),
		unbondingTime,
		unbondingValue,
		params.SlashingPkScript,
		params.SlashingRate,
		unbondingTime,
	)

	unbondingTxMsg := testUnbondingInfo.UnbondingTx

	unbondingSlashingPathInfo, err := testUnbondingInfo.UnbondingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)

	unbondingSig, err := testUnbondingInfo.SlashingTx.Sign(
		unbondingTxMsg,
		datagen.StakingOutIdx,
		unbondingSlashingPathInfo.GetPkScriptPath(),
		delBtcPrivKey,
	)
	require.NoError(t, err)

	serializedUnbondingTx, err := bbntypes.SerializeBTCTx(testUnbondingInfo.UnbondingTx)
	require.NoError(t, err)

	// Serialize the funding transaction for the stake expansion
	serializedFundingTx, err := bbntypes.SerializeBTCTx(fundingTx)
	require.NoError(t, err)

	// submit the BTC stake expansion delegation to Babylon
	_, err = tm.CovBBNClient.CreateStakeExpansionDelegation(
		bbntypes.NewBIP340PubKeyFromBTCPK(delBtcPubKey),
		fpPks,
		pop,
		uint32(stakingTime),
		stakingAmount,
		txInfo,
		testStakingInfo.SlashingTx,
		delegatorSig,
		serializedUnbondingTx,
		uint32(unbondingTime),
		unbondingValue,
		testUnbondingInfo.SlashingTx,
		unbondingSig,
		previousStakingTxHash,
		serializedFundingTx,
	)
	require.NoError(t, err)

	t.Log("successfully submitted a BTC stake expansion delegation")

	return &TestDelegationData{
		DelegatorPrivKey: delBtcPrivKey,
		DelegatorKey:     delBtcPubKey,
		FpPks:            fpPks,
		StakingTx:        testStakingInfo.StakingTx,
		SlashingTx:       testStakingInfo.SlashingTx,
		StakingTxInfo:    txInfo,
		DelegatorSig:     delegatorSig,
		SlashingPkScript: params.SlashingPkScript,
		StakingTime:      stakingTime,
		StakingAmount:    stakingAmount,
	}
}

func defaultBBNConfigWithKey(key, keydir string) *covcfg.BBNConfig {
	bbnCfg := covcfg.DefaultBBNConfig()
	bbnCfg.Key = key
	bbnCfg.KeyDirectory = keydir
	bbnCfg.GasAdjustment = 20

	return &bbnCfg
}

func defaultCovenantConfig(homeDir string) *covcfg.Config {
	cfg := covcfg.DefaultConfigWithHomePath(homeDir)
	cfg.BabylonConfig.KeyDirectory = homeDir

	return &cfg
}
